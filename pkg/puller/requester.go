package puller

func RequestFunc(done chan struct{}, in chan EventWrapper) bool {
	for event := range in {
		select {
		default:
			e := request(event)
			if e == nil {
				return false
			} else {
				return true
			}
		case <-done:
			return false
		}
	}
	return false
}

func request(event EventWrapper) *EventWrapper {
	// TODO: Write request logic for load / unload
	// Currently just testing my_model
	if event.ModelName == "my_model" {
		return &event
	}
	return nil
}
