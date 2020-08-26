package agent

import (
	"fmt"
	"github.com/kubeflow/kfserving/pkg/agent/storage"
	"log"
	"path/filepath"
)

type Puller struct {
	// TODO: Should this be a syncmap.Map?
	ChannelMap map[string]Channel
	Downloader Downloader
}

type Channel struct {
	EventChannel chan EventWrapper
}

func (p *Puller) AddModel(modelName string) Channel {
	// TODO: Figure out the appropriate buffer-size for this
	// TODO: Check if event Channel exists
	eventChannel := make(chan EventWrapper, 20)
	channel := Channel{
		EventChannel: eventChannel,
	}
	go p.modelProcessor(modelName, channel.EventChannel)
	p.ChannelMap[modelName] = channel
	return p.ChannelMap[modelName]
}

func (p *Puller) RemoveModel(modelName string) error {
	channel, ok := p.ChannelMap[modelName]
	if ok {
		close(channel.EventChannel)
		delete(p.ChannelMap, modelName)
	}
	if err := storage.RemoveDir(filepath.Join(p.Downloader.ModelDir, modelName)); err != nil {
		return fmt.Errorf("failing to delete model directory: %v", err)
	}
	return nil
}

func (p *Puller) modelProcessor(modelName string, events chan EventWrapper) {
	log.Println("worker for", modelName, "is initialized")
	// TODO: Instead of going through each event, one-by-one, we need to drain and combine
	// this is important for handling Load --> Unload requests sent in tandem
	// Load --> Unload = 0 (cancel first load)
	// Load --> Unload --> Load = 1 Load (cancel second load?)
	for event := range events {
		log.Println("worker", modelName, "started  job", event)
		switch event.LoadState {
		case ShouldLoad:
			log.Println("Should download", event.ModelSpec.StorageURI)
			err := p.Downloader.DownloadModel(event)
			if err != nil {
				log.Println("worker failed on", event, "because: ", err)
			} else {
				// If there is an error, we will NOT send a request. As such, to know about errors, you will
				// need to call the error endpoint of the puller
				// TODO: Do request logic
				log.Println("Now doing a request on", event)
			}
		case ShouldUnload:
			// If there is an error, we will NOT send a request. As such, to know about errors, you will
			// need to call the error endpoint of the puller
			// TODO: Do request logic
			log.Println("Now doing a request on", event)
			log.Println("Should unload", event.ModelName)
			if err := p.RemoveModel(event.ModelName); err != nil {
				log.Println("worker failed on", event, "because: ", err)
			}
		}
	}
}
