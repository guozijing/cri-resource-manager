// Copyright 2029 Intel Corporation. All Rights Reserved.
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

//go:build test
// +build test

package resmgr

import (
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
)

// ResourceManagerTestAPI is a post-test verification helper interface.
type ResourceManagerTestAPI interface {
	// GetCache returns the Cache resource manager is running with.
	GetCache() cache.Cache
}

func (m *resmgr) GetCache() cache.Cache {
	return m.cache
}
