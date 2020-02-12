// Copyright 2019 The Knative Authors

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or im
// See the License for the specific language governing permissions and
// limitations under the License.

// +build e2e
// +build !eventing

package e2e

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"gotest.tools/assert"
	"knative.dev/client/pkg/util"
)

func TestRevision(t *testing.T) {
	t.Parallel()
	test := NewE2eTest(t)
	test.Setup(t)
	defer test.Teardown(t)

	t.Run("create hello service and return no error", func(t *testing.T) {
		test.serviceCreate(t, "hello")
	})

	t.Run("describe revision from hello service with print flags", func(t *testing.T) {
		revName := test.findRevision(t, "hello")
		test.revisionDescribeWithPrintFlags(t, revName)
	})

	t.Run("update hello service and increase the count of configuration generation", func(t *testing.T) {
		test.serviceUpdate(t, "hello", []string{"--env", "TARGET=kn", "--port", "8888"})
	})

	t.Run("show a list of revisions sorted by the count of configuration generation", func(t *testing.T) {
		test.revisionListWithService(t, "hello")
	})

	t.Run("delete latest revision from hello service and return no error", func(t *testing.T) {
		revName := test.findRevision(t, "hello")
		test.revisionDelete(t, revName)
	})

	t.Run("delete three revisions with one revision a nonexistent", func(t *testing.T) {
		// increase count to 2 revisions
		test.serviceUpdate(t, "hello", []string{"--env", "TARGET=kn", "--port", "8888"})

		existRevision1 := test.findRevisionByGeneration(t, "hello", 1)
		existRevision2 := test.findRevisionByGeneration(t, "hello", 2)
		nonexistRevision := "hello-nonexist"

		test.revisionMultipleDelete(t, []string{existRevision1, existRevision2, nonexistRevision})
	})

	t.Run("delete hello service and return no error", func(t *testing.T) {
		test.serviceDelete(t, "hello")
	})
}

func (test *e2eTest) revisionListWithService(t *testing.T, serviceNames ...string) {
	for _, svcName := range serviceNames {
		confGen := test.findConfigurationGeneration(t, svcName)

		out, err := test.kn.RunWithOpts([]string{"revision", "list", "-s", svcName}, runOpts{})
		assert.NilError(t, err)

		outputLines := strings.Split(out, "\n")
		// Ignore the last line because it is an empty string caused by splitting a line break
		// at the end of the output string
		for _, line := range outputLines[1 : len(outputLines)-1] {
			revName := test.findRevisionByGeneration(t, svcName, confGen)
			assert.Check(t, util.ContainsAll(line, revName, svcName, strconv.Itoa(confGen)))
			confGen--
		}
	}
}

func (test *e2eTest) revisionDelete(t *testing.T, revName string) {
	out, err := test.kn.RunWithOpts([]string{"revision", "delete", revName}, runOpts{})
	assert.NilError(t, err)

	assert.Check(t, util.ContainsAll(out, "Revision", revName, "deleted", "namespace", test.kn.namespace))
}

func (test *e2eTest) revisionMultipleDelete(t *testing.T, revisionNames []string) {
	existRevision1 := revisionNames[0]
	existRevision2 := revisionNames[1]
	nonexistRevision := revisionNames[2]
	out, err := test.kn.RunWithOpts([]string{"revision", "list"}, runOpts{NoNamespace: false})
	assert.NilError(t, err)
	assert.Check(t, strings.Contains(out, existRevision1), "Required revision1 does not exist")
	assert.Check(t, strings.Contains(out, existRevision2), "Required revision2 does not exist")
	assert.Check(t, !strings.Contains(out, nonexistRevision), "Nonexistent revision does exist")

	out, err = test.kn.RunWithOpts([]string{"revision", "delete", existRevision1, existRevision2, nonexistRevision}, runOpts{NoNamespace: false})

	// expectedSuccess1 := fmt.Sprintf(`Revision '%s' successfully deleted in namespace '%s'`, existRevision1, test.kn.namespace)
	// expectedSuccess2 := fmt.Sprintf(`Revision '%s' successfully deleted in namespace '%s'`, existRevision2, test.kn.namespace)
	// expectedErr := fmt.Sprintf(`revisions.serving.knative.dev "%s" not found.`, nonexistRevision)

	// assert.Check(t, strings.Contains(out, expectedSuccess1), "Failed to get 'successfully deleted' first revision message")
	// assert.Check(t, strings.Contains(out, expectedSuccess2), "Failed to get 'successfully deleted' second revision message")
	// assert.Check(t, strings.Contains(out, expectedErr), "Failed to get 'not found' error")

	assert.Check(t, util.ContainsAll(out, "Revision", existRevision1, "successfully", "deleted", "namespace", test.kn.namespace), "Failed to get 'successfully deleted' first revision message")
	assert.Check(t, util.ContainsAll(out, "Revision", existRevision2, "successfully", "deleted", "namespace", test.kn.namespace), "Failed to get 'successfully deleted' second revision message")
	assert.Check(t, util.ContainsAll(out, "revisions.serving.knative.dev", nonexistRevision, "not found"), "Failed to get 'not found' error")
}

func (test *e2eTest) revisionDescribeWithPrintFlags(t *testing.T, revName string) {
	out, err := test.kn.RunWithOpts([]string{"revision", "describe", revName, "-o=name"}, runOpts{})
	assert.NilError(t, err)

	expectedName := fmt.Sprintf("revision.serving.knative.dev/%s", revName)
	assert.Equal(t, strings.TrimSpace(out), expectedName)
}

func (test *e2eTest) findRevision(t *testing.T, serviceName string) string {
	revName, err := test.kn.RunWithOpts([]string{"revision", "list", "-s", serviceName, "-o=jsonpath={.items[0].metadata.name}"}, runOpts{})
	assert.NilError(t, err)
	if strings.Contains(revName, "No resources found.") {
		t.Errorf("Could not find revision name.")
	}
	return revName
}

func (test *e2eTest) findRevisionByGeneration(t *testing.T, serviceName string, generation int) string {
	maxGen := test.findConfigurationGeneration(t, serviceName)
	revName, err := test.kn.RunWithOpts([]string{"revision", "list", "-s", serviceName,
		fmt.Sprintf("-o=jsonpath={.items[%d].metadata.name}", maxGen-generation)}, runOpts{})
	assert.NilError(t, err)
	if strings.Contains(revName, "No resources found.") {
		t.Errorf("Could not find revision name.")
	}
	return revName
}

func (test *e2eTest) findConfigurationGeneration(t *testing.T, serviceName string) int {
	confGenStr, err := test.kn.RunWithOpts([]string{"revision", "list", "-s", serviceName, "-o=jsonpath={.items[0].metadata.labels.serving\\.knative\\.dev/configurationGeneration}"}, runOpts{})
	assert.NilError(t, err)
	if confGenStr == "" {
		t.Errorf("Could not find configuration generation.")
	}
	confGen, err := strconv.Atoi(confGenStr)
	if err != nil {
		t.Errorf("Invalid type of configuration generation: %s", err)
	}

	return confGen
}
