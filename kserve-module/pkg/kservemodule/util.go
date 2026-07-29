package kservemodule

func configOrDefault(configData map[string]string, key, defaultVal string) string {
	if v, ok := configData[key]; ok && v != "" {
		return v
	}
	return defaultVal
}
