/*

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

package v1alpha2

import (
	"github.com/onsi/gomega"
	"testing"
)

func TestLoggerValidate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// OK
	il0 := Logger{Mode: LogAll}
	err := validateLogger(&il0)
	g.Expect(err).To(gomega.BeNil())

	url := "http://localhost"
	// OK
	il1 := Logger{Url: &url, Mode: LogAll}
	err = validateLogger(&il1)
	g.Expect(err).To(gomega.BeNil())

	// Invalid logger type
	il2 := Logger{Url: &url, Mode: "a"}
	err = validateLogger(&il2)
	g.Expect(err).NotTo(gomega.BeNil())
	g.Expect(err.Error()).To(gomega.Equal(InvalidLoggerType))

}
