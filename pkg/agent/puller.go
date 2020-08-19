package agent

import (
	"log"
)

type Puller struct {
	// TODO: Should this be a syncmap?
	ChannelMap map[string]Channel
	Downloader Downloader
}

type Channel struct {
	EventChannel chan EventWrapper
}

func (p *Puller) AddModel(modelName string, numWorkers int) Channel {
	// TODO: Figure out the appropriate buffer-size for this
	// TODO: Check if event Channel exists
	eventChannel := make(chan EventWrapper)
	channel := Channel{
		EventChannel: eventChannel,
	}
	// TODO: Figure out parallelism
	for workers := 1; workers <= numWorkers; workers++ {
		go p.modelProcessor(workers, modelName, channel.EventChannel)
	}
	p.ChannelMap[modelName] = channel
	return p.ChannelMap[modelName]
}

// TODO: Hook up this to the Unload method
func (p *Puller) RemoveModel(modelName string) {
	channel, ok := p.ChannelMap[modelName]
	if ok {
		close(channel.EventChannel)
		delete(p.ChannelMap, modelName)
	}
}

func (p * Puller) modelProcessor(id int, modelName string, events <-chan EventWrapper) {
	log.Println("worker", id, "for", modelName, "is initialized")
	var err error
	// TODO: Instead of going through each event, one-by-one, we need to drain and combine
	// this is important for handling Load --> Unload requests sent in tandem
	// Load --> Unload = 0 (cancel first load)
	// Load --> Unload --> Load = 1 Load (cancel second load?)
	for event := range events {
		log.Println("worker", id, modelName, "started  job", event)
		switch event.LoadState {
		case ShouldLoad:
			log.Println("Should download", event.ModelSpec.StorageURI)
			err = p.Downloader.DownloadModel(event)
			if err != nil {
				log.Println("worker", id, "failed on", event, "because: ", err)
			}
		}
		// If there is an error, we will NOT send a request. As such, to know about errors, you will
		// need to call the error endpoint of the puller
		if err == nil {
			log.Println("Now doing a request on", event)
		}
		//innerErr := RequestModel(event)
		//if innerErr != nil {
		//	log.Println("worker", id, "failed on", event, "because: ", err)
		//} else {
		//	log.Println("worker", id, "finished  job", event)
		//}
	}
}
