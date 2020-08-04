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
	NumRetries int
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
		var err error
		switch event.LoadState {
		case ShouldLoad:
			err = DownloadModel(p.NumRetries, event)
			if err != nil {
				log.Println("worker", id, "failed on", event, "because: ", err)
			}
		}
		if err == nil {
			innerErr := RequestModel(event)
			if innerErr != nil {
				log.Println("worker", id, "failed on", event, "because: ", err)
			} else {
				log.Println("worker", id, "finished  job", event)
			}
		}
	}
}

func AddModelToChannel(e EventWrapper) {
	p.Channel <- e
}

func InitiatePullers(numWorkers int, numRetries int) {
	p.NumWorkers = numWorkers
	p.NumRetries = numRetries
	p.Channel = make(chan EventWrapper)
	for workers := 1; workers <= p.NumWorkers; workers++ {
		go worker(workers, p.Channel)
	}
}
