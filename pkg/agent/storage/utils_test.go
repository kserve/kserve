/*
Copyright 2021 The KServe Authors.

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

package storage

import (
	"os"
	"path"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	awss3 "github.com/aws/aws-sdk-go/service/s3"
	"github.com/onsi/gomega"

	"github.com/kserve/kserve/pkg/agent/mocks"
	s3credential "github.com/kserve/kserve/pkg/credentials/s3"
)

func TestCreate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// This would get called in StartPullerAndProcessModels
	syscall.Umask(0)

	tmpDir := t.TempDir()
	folderPath := path.Join(tmpDir, "foo")
	filePath := path.Join(folderPath, "bar.txt")
	f, err := Create(filePath)
	if err != nil {
		t.Fatalf("Unable to create file: %v", err)
	}
	defer func() { _ = f.Close() }()

	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(folderPath).To(gomega.BeADirectory())

	info, _ := os.Stat(folderPath)
	mode := info.Mode()
	expectedMode := os.ModePerm
	g.Expect(mode.Perm()).To(gomega.Equal(expectedMode))
}

func TestFileExists(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	syscall.Umask(0)
	tmpDir := t.TempDir()

	// Test case for existing file
	f, err := os.CreateTemp(tmpDir, "tmpfile")
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(FileExists(f.Name())).To(gomega.BeTrue())
	_ = f.Close()

	// Test case for not existing file
	path := filepath.Join(tmpDir, "fileNotExist")
	g.Expect(FileExists(path)).To(gomega.BeFalse())

	// Test case for directory as filename
	g.Expect(FileExists(tmpDir)).To(gomega.BeFalse())
}

func TestRemoveDir(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	syscall.Umask(0)
	tmpDir := t.TempDir()

	// Create a subdirectory within tmpDir
	subDir := filepath.Join(tmpDir, "test")
	err := os.Mkdir(subDir, 0o755) //nolint:gosec // test directory permissions are not security-sensitive
	g.Expect(err).ToNot(gomega.HaveOccurred())

	f, err := os.CreateTemp(subDir, "tmp")
	if err != nil {
		t.Fatalf("os.CreateTemp failed: %v", err)
	}
	defer f.Close()

	_, _ = os.CreateTemp(tmpDir, "tmp")

	err = RemoveDir(tmpDir)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	_, err = os.Stat(tmpDir)
	g.Expect(os.IsNotExist(err)).To(gomega.BeTrue())

	// Test case for non existing directory
	err = RemoveDir("directoryNotExist")
	g.Expect(err).To(gomega.HaveOccurred())
}

func TestGetProvider(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// When providers map already have specified provider
	mockProviders := map[Protocol]Provider{
		S3: &S3Provider{
			Client:     &mocks.MockS3Client{},
			Downloader: &mocks.MockS3Downloader{},
		},
		GCS: &GCSProvider{
			Client: mocks.NewMockClient(),
		},
		AZURE: &AzureProvider{
			Client: mocks.NewMockAzureClient(),
		},
	}
	for nextProtocol := range mockProviders {
		provider, err := GetProvider(mockProviders, nextProtocol)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(provider).ShouldNot(gomega.BeNil())
		g.Expect(provider).Should(gomega.Equal(mockProviders[nextProtocol]))

		// When providers map does not have specified provider
		for _, protocol := range SupportedProtocols {
			provider, err = GetProvider(map[Protocol]Provider{}, protocol)
			g.Expect(err).ToNot(gomega.HaveOccurred())
			g.Expect(provider).ShouldNot(gomega.BeNil())
		}
	}
}

func TestGetProviderS3ConfigParsesNumericBoolEnvs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	t.Setenv(s3credential.S3UseVirtualBucket, "0")
	t.Setenv(s3credential.S3UseAccelerate, "1")
	t.Setenv(s3credential.AWSAnonymousCredential, "1")

	provider, err := GetProvider(map[Protocol]Provider{}, S3)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	s3Provider, ok := provider.(*S3Provider)
	g.Expect(ok).To(gomega.BeTrue())
	s3Client, ok := s3Provider.Client.(*awss3.S3)
	g.Expect(ok).To(gomega.BeTrue())

	g.Expect(aws.BoolValue(s3Client.Config.S3ForcePathStyle)).To(gomega.BeTrue())
	g.Expect(aws.BoolValue(s3Client.Config.S3UseAccelerate)).To(gomega.BeTrue())
	g.Expect(
		s3Client.Config.Credentials == credentials.AnonymousCredentials,
	).To(gomega.BeTrue())
}

func TestParseBoolEnvOrDefault(t *testing.T) {
	scenarios := map[string]struct {
		set          bool
		value        string
		defaultValue bool
		expected     bool
	}{
		"UnsetUsesDefaultTrue": {
			defaultValue: true,
			expected:     true,
		},
		"UnsetUsesDefaultFalse": {
			defaultValue: false,
			expected:     false,
		},
		"EmptyUsesDefault": {
			set:          true,
			defaultValue: true,
			expected:     true,
		},
		"ZeroParsesFalse": {
			set:          true,
			value:        "0",
			defaultValue: true,
			expected:     false,
		},
		"OneParsesTrue": {
			set:          true,
			value:        "1",
			defaultValue: false,
			expected:     true,
		},
		"UpperFalseParsesFalse": {
			set:          true,
			value:        "FALSE",
			defaultValue: true,
			expected:     false,
		},
		"UpperTrueParsesTrue": {
			set:          true,
			value:        "TRUE",
			defaultValue: false,
			expected:     true,
		},
		"ShortFalseParsesFalse": {
			set:          true,
			value:        "f",
			defaultValue: true,
			expected:     false,
		},
		"ShortTrueParsesTrue": {
			set:          true,
			value:        "t",
			defaultValue: false,
			expected:     true,
		},
		"InvalidUsesDefaultTrue": {
			set:          true,
			value:        "invalid",
			defaultValue: true,
			expected:     true,
		},
		"InvalidUsesDefaultFalse": {
			set:          true,
			value:        "invalid",
			defaultValue: false,
			expected:     false,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)
			const envVar = "KSERVE_TEST_S3_BOOL"
			if scenario.set {
				t.Setenv(envVar, scenario.value)
			} else {
				_ = os.Unsetenv(envVar)
			}

			g.Expect(
				parseBoolEnvOrDefault(envVar, scenario.defaultValue),
			).To(gomega.Equal(scenario.expected))
		})
	}
}
