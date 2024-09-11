package main

import (
	"log"
	"os"
	"time"

	"github.com/dnstapir/tapir"
)

type Logger struct {
	file *os.File
	log  *log.Logger
}

func NewLogger(filename string) *Logger {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Error opening log file: %v", err)
	}

	return &Logger{
		file: file,
		log:  log.New(file, "", log.LstdFlags|log.Lshortfile),
	}
}

func (l *Logger) LogStatus(edgeId, edgeComponent string, status tapir.TapirFunctionStatus) {
	l.log.Printf("Status update from TAPIR Edge (id: %s, component: %s)", edgeId, edgeComponent)
	for _, comp := range status.ComponentStatus {
		switch comp.Status {
		case "fail":
			l.log.Printf("TAPIR-POP %s Component: %s, Status: %s, Message: %s, Time of failure: %s",
				status.FunctionID, comp.Component, comp.Status, comp.Msg, comp.LastFail.Format(time.RFC3339))
		case "warn":
			l.log.Printf("TAPIR-POP %s: Component: %s, Status: %s, Message: %s, Time of warning: %s",
				status.FunctionID, comp.Component, comp.Status, comp.Msg, comp.LastWarn.Format(time.RFC3339))
		case "success":
			l.log.Printf("TAPIR-POP %s Component: %s, Status: %s, Message: %s, Time of success: %s",
				status.FunctionID, comp.Component, comp.Status, comp.Msg, comp.LastSuccess.Format(time.RFC3339))
		}
	}
}

func (l *Logger) Close() {
	l.file.Close()
}
