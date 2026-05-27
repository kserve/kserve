/*
Copyright 2026 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	logging "github.com/sirupsen/logrus"
)

var (
	instance *Config
	once     sync.Once
)

type MCVConfig struct {
	MCVNamespace     string
	EnabledGPU       *bool
	EnabledStub      *bool
	KubeConfig       string
	EnabledBaremetal *bool
	SkipPrecheck     *bool
	Timeout          int
}

type Config struct {
	KernelVersion float32
	MCV           MCVConfig
	ConfDir       string
}

func newConfig(baseDir string) (*Config, error) {
	if baseDir == "" {
		baseDir = defaultConfDir
	}
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for config-dir: %s: %w", baseDir, err)
	}

	s, err := os.Stat(absBaseDir)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(absBaseDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create config-dir %s: %w", absBaseDir, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat config-dir %s: %w", absBaseDir, err)
	} else if !s.IsDir() {
		return nil, fmt.Errorf("config-dir %s is not a directory", absBaseDir)
	}

	return &Config{
		ConfDir:       absBaseDir,
		MCV:           getMCVConfig(absBaseDir),
		KernelVersion: 0,
	}, nil
}

func Initialize(baseDir string) (*Config, error) {
	var err error
	once.Do(func() {
		instance, err = newConfig(baseDir)
	})
	return instance, err
}

func Instance() *Config {
	return instance
}

func IsInitialized() bool {
	return instance != nil
}

func getMCVConfig(confDir string) MCVConfig {
	return MCVConfig{
		EnabledGPU:       parseBoolEnv(envEnableGPU, true),
		SkipPrecheck:     parseBoolEnv(envSkipPrecheck, false),
		EnabledBaremetal: parseBoolEnv(envEnableBaremetal, false),
		EnabledStub:      parseBoolEnv(envEnableSTUB, false),
		MCVNamespace:     getConfig(envMCVNamespace, defaultNamespace, confDir),
		KubeConfig:       getConfig(envKubeConfig, defaultKubeConfig, confDir),
		Timeout:          10, // TODO should add a max timeout check
	}
}

func parseBoolEnv(key string, defaultVal bool) *bool {
	if val, exists := os.LookupEnv(key); exists {
		b := strings.EqualFold(val, "true")
		return &b
	}
	return &defaultVal
}

func getConfig(key, defaultValue, confDir string) string {
	if envValue, exists := os.LookupEnv(key); exists {
		return envValue
	}
	configFile := filepath.Join(confDir, key)
	if value, err := os.ReadFile(configFile); err == nil {
		return strings.TrimSpace(bytes.NewBuffer(value).String())
	}
	return defaultValue
}

func logBoolConfigs() {
	logging.Infof("ENABLE_GPU: %t", IsGPUEnabled())
	logging.Infof("ENABLE_BAREMETAL: %t", IsBaremetalEnabled())
	logging.Infof("ENABLE_STUB: %t", IsStubEnabled())
}

func LogConfigs() {
	logging.Infof("config-dir: %s", instance.ConfDir)
	logBoolConfigs()
}

func SetEnabledGPU(enabled bool) {
	b := enabled
	instance.MCV.EnabledGPU = &b
}

func SetEnabledStub(enabled bool) {
	b := enabled
	instance.MCV.EnabledStub = &b
}

func SetTimeout(timeout int) {
	instance.MCV.Timeout = timeout
}

func SetSkipPrecheck(enabled bool) {
	b := enabled
	instance.MCV.SkipPrecheck = &b
}

func SetEnabledBaremetal(enabled bool) {
	b := enabled
	instance.MCV.EnabledBaremetal = &b
}

func SetKubeConfig(k string) {
	instance.MCV.KubeConfig = k
}

func KubeConfig() string {
	return instance.MCV.KubeConfig
}

func IsGPUEnabled() bool {
	return instance.MCV.EnabledGPU != nil && *instance.MCV.EnabledGPU
}

func IsStubEnabled() bool {
	return instance.MCV.EnabledStub != nil && *instance.MCV.EnabledStub
}

func IsSkipPrecheckEnabled() bool {
	return instance.MCV.SkipPrecheck != nil && *instance.MCV.SkipPrecheck
}

func IsBaremetalEnabled() bool {
	return instance.MCV.EnabledBaremetal != nil && *instance.MCV.EnabledBaremetal
}

func Timeout() int {
	return instance.MCV.Timeout
}
