package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"godvr/internal/dvrip"
)

var (
	address       = flag.String("address", "192.168.1.147", "camera address: 192.168.1.147, 192.168.1.147:34567")
	name          = flag.String("name", "camera1", "name of the camera")
	outPath       = flag.String("out", "./", "output path that video files will be kept")
	chunkInterval = flag.Duration("chunkInterval", time.Minute*10, "time when application must create a new files")
	stream        = flag.String("stream", "Main", "camera stream name")
	user          = flag.String("user", "admin", "username")
	password      = flag.String("password", "password", "password")
	retryTime     = flag.Duration("retryTime", time.Minute, "retry to connect if problem occur")
)

func main() {
	flag.Parse()

	settings := dvrip.Settings{
		Address:  *address,
		User:     *user,
		Password: *password,
	}

	err := setupLogs()
	if err != nil {
		log.Print("WARNING: failed to setup log file:", err)
	}

	settings.SetDefaults()
	log.Printf("DEBUG: using the following settings: %+v", settings)

	for {
		err := monitor(settings)
		if err == nil {
			break
		}

		log.Print("fatal error: ", err)
		log.Printf("wait %v and try again", *retryTime)

		time.Sleep(*retryTime)
	}
}

func monitor(settings dvrip.Settings) error {
	conn, err := dvrip.New(settings)
	if err != nil {
		log.Printf("failed to initiatiate connection: %v", err)
		return err
	}

	err = conn.Login()
	if err != nil {
		log.Panic("failed to login", err)
		return err
	}

	log.Print("DEBUG: successfully logged in")

	err = conn.SetKeepAlive()
	if err != nil {
		log.Print("failed to set keepalive:", err)
		return err
	}

	log.Print("DEBUG: successfully set keepalive")

	err = conn.SetTime()
	if err != nil {
		log.Print("failed to set time:", err)
		return err
	}

	log.Print("DEBUG: successfully synced time")

	outChan := make(chan *dvrip.Frame)
	var videoFile, audioFile *os.File

	err = conn.Monitor(*stream, outChan)
	if err != nil {
		log.Print("failed to start monitoring:", err)
		return err
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, os.Kill)

	videoFile, audioFile, err = createChunkFiles(time.Now())
	if err != nil {
		return err
	}

	prevTime := time.Now()

	for {
		select {
		case frame, ok := <-outChan:
			if !ok {
				return conn.MonitorErr
			}

			now := time.Now()

			if prevTime.Add(*chunkInterval).After(now) {
				errs := closeFiles(videoFile, audioFile)
				if err != nil {
					log.Printf("error occurred: %v", errs)
				}

				videoFile, audioFile, err = createChunkFiles(now)
				prevTime = now
			}

			err = processFrame(frame, audioFile, videoFile)
			if err != nil {
				log.Println("failed to process the frame", err)
				return err
			}
		case <-stop:
			log.Println("received interrupt signal: stopping")

			errs := closeFiles(videoFile, audioFile)
			if err != nil {
				log.Printf("error occurred: %v", errs)
			}

			return nil
		}
	}

	return nil
}

func processFrame(frame *dvrip.Frame, audioFile, videoFile *os.File) error {
	if frame.Meta.Type == "G711A" { // audio
		_, err := audioFile.Write(frame.Data)
		if err != nil {
			log.Println("WARNING: failed to write to file", err)
		}

		return nil
	}

	if frame.Meta.Frame != "" {
		_, err := videoFile.Write(frame.Data)
		if err != nil {
			log.Println("WARNING: failed to write to file", err)
		}

		return nil
	}

	return nil // TODO
}

func closeFiles(files ...*os.File) (errs []error) {
	for _, f := range files {
		err := f.Close()
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to close file: %v cause: %v", f.Name(), err))
		}
	}

	return
}

func createChunkFiles(t time.Time) (*os.File, *os.File, error) {
	dir := *outPath + "/" + (*name) + t.Format("/2006/01/02/")

	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return nil, nil, err
	}

	file := *outPath + "/" + (*name) + t.Format("/2006/01/02/15.04.05")
	log.Print("starting files:", file)

	videoFile, err := os.Create(file + ".video")
	if err != nil {
		return nil, nil, err
	}

	audioFile, err := os.Create(file + ".audio")
	if err != nil {
		return nil, nil, err
	}

	return videoFile, audioFile, nil
}

func setupLogs() error {
	outDir := *outPath + "/" + *name
	err := os.Mkdir(outDir, os.ModeDir)

	if err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}

	logsFile, err := os.OpenFile(outDir+"/"+"logs.log", os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
	if err != nil {
		return err
	}

	log.SetOutput(logsFile)

	return nil
}
