package puller

import "log"

func DownloadFunc(done chan struct{}, in chan EventWrapper) chan EventWrapper {
	out := make(chan EventWrapper)
	go func() {
		for event := range in {
			select {
			default:
				e := download(event)
				if e == nil {
					if event.DownloadRetries > 2 {
						// LOOK AT THIS
						close(done)
						close(out)
					} else {
						log.Println("I am retrying", event.ModelName)
						event.DownloadRetries += 1
						in <- event
					}
				} else {
					out <- event
				}
			case <-done:
				return
			}
		}
		close(out)
	}()
	return out
}

func download(event EventWrapper) *EventWrapper {
	// TODO: Proper download logic
	// We are testing the retry logic here
	if event.ModelName == "my_model" || event.ModelName == "my_model2"{
		log.Println("Downloading model,", event.ModelName)
		if event.DownloadRetries == 0 {
			log.Println("Need to retry download", event.ModelName)
			return nil
		}
		log.Println("Success downloading my model", event.ModelName)
		return &event
	}
	return nil
}
