package puller

import (
	"log"
)

var (
	p *Puller
)


type Puller struct {
	NumWorkers int
	Channel chan EventWrapper
}

func init() {
	p = NewPuller()
}

func NewPuller() *Puller {
	p := new(Puller)
	return p
}

func worker(id int, events<-chan EventWrapper) {
	log.Println("worker", id)
	for event := range events {
		log.Println("worker", id, "started  job", event)
		// TODO: Need to signal to close done and downloadChannel
		// instead of closing in go-routines
		done := make(chan struct{})
		downloadChannel := make(chan EventWrapper, 1)
		downloadChannel <- event
		requestChannel := DownloadFunc(done, downloadChannel)
		result := RequestFunc(done, requestChannel)
		log.Println("worker", id, "finished job", event, "with:", result)
	}
}

func AddModelToChannel(e EventWrapper) {
	p.Channel <- e
}

func InitiatePullers(numWorkers int) {
	p.NumWorkers = numWorkers
	p.Channel = make(chan EventWrapper)
	for workers := 1; workers <= p.NumWorkers; workers++ {
		go worker(workers, p.Channel)
	}
}
