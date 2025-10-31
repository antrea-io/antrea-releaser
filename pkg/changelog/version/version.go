// Copyright 2025 Antrea Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package version

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
)

// Version represents a semantic version
type Version struct {
	major uint64
	minor uint64
	patch uint64
}

// Parse parses a semantic version string (X.Y.Z) using the semver library
func Parse(versionStr string) (*Version, error) {
	v, err := semver.NewVersion(versionStr)
	if err != nil {
		return nil, fmt.Errorf("invalid version %s: %w", versionStr, err)
	}
	return &Version{
		major: v.Major(),
		minor: v.Minor(),
		patch: v.Patch(),
	}, nil
}

// New creates a new Version instance with the given components
func New(major, minor, patch uint64) *Version {
	return &Version{
		major: major,
		minor: minor,
		patch: patch,
	}
}

// Major returns the major version
func (v *Version) Major() uint64 {
	return v.major
}

// Minor returns the minor version
func (v *Version) Minor() uint64 {
	return v.minor
}

// Patch returns the patch version
func (v *Version) Patch() uint64 {
	return v.patch
}

// String returns the string representation of the version
func (v *Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
}

// GreaterThan returns true if this version is greater than the other version
func (v *Version) GreaterThan(other *Version) bool {
	if v.major != other.major {
		return v.major > other.major
	}
	if v.minor != other.minor {
		return v.minor > other.minor
	}
	return v.patch > other.patch
}

// CalculatePreviousRelease calculates the previous release version
func (v *Version) CalculatePreviousRelease() string {
	if v.patch == 0 {
		// Minor release: previous minor version
		if v.minor > 0 {
			return fmt.Sprintf("%d.%d.0", v.major, v.minor-1)
		}
		// First minor version of major release
		return fmt.Sprintf("%d.0.0", v.major)
	}
	// Patch release: previous patch version
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch-1)
}
