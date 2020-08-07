package puller

import (
	"log"
)

var (
	p *Puller
)

type Puller struct {
	// TODO: Should this be a syncmap?
	ChannelMap map[string]Channel
}

type Channel struct {
	EventChannel chan EventWrapper
}

func (*Puller) AddModel(modelName string, numWorkers int) Channel {
	// TODO: Figure out the appropriate buffer-size for this
	eventChannel := make(chan EventWrapper)
	channel := Channel{
		EventChannel: eventChannel,
	}
	// TODO: Figure out parallelism
	for workers := 1; workers <= numWorkers; workers++ {
		go worker(workers, modelName, channel.EventChannel)
	}
	p.ChannelMap[modelName] = channel
	return p.ChannelMap[modelName]
}

// TODO: Hook up this to the Unload method
func (*Puller) RemoveModel(modelName string) {
	channel, ok := p.ChannelMap[modelName]
	if ok {
		close(channel.EventChannel)
		delete(p.ChannelMap, modelName)
	}
}

func init() {
	p = NewPuller()
}

func NewPuller() *Puller {
	p := new(Puller)
	p.ChannelMap = map[string]Channel{}
	return p
}

func worker(id int, modelName string, events <-chan EventWrapper) {
	log.Println("worker", id, modelName, "initialized")
	for event := range events {
		log.Println("worker", id, modelName, "started  job", event)
		switch event.LoadState {
		case ShouldLoad:
			log.Println("Should download", event)
			//err = DownloadModel(p.NumRetries, event)
			//if err != nil {
			//	log.Println("worker", id, "failed on", event, "because: ", err)
			//}
		}
		log.Println("Now doing a request on", event)
		//innerErr := RequestModel(event)
		//if innerErr != nil {
		//	log.Println("worker", id, "failed on", event, "because: ", err)
		//} else {
		//	log.Println("worker", id, "finished  job", event)
		//}
	}
}
