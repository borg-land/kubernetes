#!/usr/bin/env bash

# Copyright 2016 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Generate root CA
step certificate create "Root-CA" root.pem root-key.pem \
    --profile root-ca \
    --no-password --insecure --force \
    --not-after=876000h

# Generate intermediate CA
step certificate create "Intermediate-CA" intermediate.pem intermediate-key.pem \
    --profile intermediate-ca \
    --ca root.pem --ca-key root-key.pem \
    --no-password --insecure --force \
    --not-after=876000h

# Generate valid client certificate
step certificate create "My Client" client-valid.pem client-valid-key.pem \
    --ca intermediate.pem --ca-key intermediate-key.pem \
    --no-password --insecure --force \
    --not-after=876000h

# Generate expired client certificate
step certificate create "My Client" client-expired.pem client-expired-key.pem \
    --ca intermediate.pem --ca-key intermediate-key.pem \
    --no-password --insecure --force \
    --not-before="1990-12-31T23:59:00Z" \
    --not-after="1990-12-31T23:59:00Z"

