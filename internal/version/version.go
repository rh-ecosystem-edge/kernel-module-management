/*
Copyright 2022.

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

package version

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

var ocpVersionRe = regexp.MustCompile(`^v?(\d+)\.(\d+)`)

// OCPVersion holds the parsed major and minor version of an OpenShift cluster.
type OCPVersion struct {
	Major int
	Minor int
}

// AtLeast returns true if ov is greater than or equal to the given major.minor version.
func (ov OCPVersion) AtLeast(major, minor int) bool {
	return ov.Major > major || (ov.Major == major && ov.Minor >= minor)
}

// DiscoverOCPVersion queries the OpenShift ClusterVersion resource and returns the cluster version.
func DiscoverOCPVersion(cfg *rest.Config) (OCPVersion, error) {
	ocpScheme := runtime.NewScheme()
	if err := configv1.Install(ocpScheme); err != nil {
		return OCPVersion{}, fmt.Errorf("failed to register OpenShift config scheme: %v", err)
	}

	cfgCopy := *cfg
	cfgCopy.GroupVersion = &schema.GroupVersion{Group: "config.openshift.io", Version: "v1"}
	cfgCopy.APIPath = "/apis"
	cfgCopy.NegotiatedSerializer = serializer.NewCodecFactory(ocpScheme)

	client, err := rest.RESTClientFor(&cfgCopy)
	if err != nil {
		return OCPVersion{}, fmt.Errorf("failed to create REST client for OpenShift config API: %v", err)
	}

	cv := &configv1.ClusterVersion{}
	if err = client.Get().Resource("clusterversions").Name("version").Do(context.Background()).Into(cv); err != nil {
		return OCPVersion{}, fmt.Errorf("failed to get ClusterVersion: %v", err)
	}

	return ParseOCPVersion(cv.Status.Desired.Version)
}

// ParseOCPVersion extracts the major and minor version from an OpenShift
// version string such as "4.21.0" or "4.21.0-rc.1".
func ParseOCPVersion(version string) (OCPVersion, error) {
	m := ocpVersionRe.FindStringSubmatch(version)
	if m == nil {
		return OCPVersion{}, fmt.Errorf("cannot parse OpenShift version from %q", version)
	}

	major, err := strconv.Atoi(m[1])
	if err != nil {
		return OCPVersion{}, fmt.Errorf("cannot parse OpenShift major version from %q: %v", version, err)
	}
	minor, err := strconv.Atoi(m[2])
	if err != nil {
		return OCPVersion{}, fmt.Errorf("cannot parse OpenShift minor version from %q: %v", version, err)
	}
	return OCPVersion{Major: major, Minor: minor}, nil
}
