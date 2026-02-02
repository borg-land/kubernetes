#!/usr/bin/env bash

# Copyright 2021 The Kubernetes Authors.
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

# Generate client CA
step certificate create "client-ca" client-ca.pem client-ca-key.pem \
    --profile root-ca \
    --no-password --insecure --force \
    --not-after=876000h

# Generate server CA
step certificate create "server-ca" server-ca.pem server-ca-key.pem \
    --profile root-ca \
    --no-password --insecure --force \
    --not-after=876000h

# Generate client certificate
step certificate create "My Client" client.pem client-key.pem \
    --ca client-ca.pem --ca-key client-ca-key.pem \
    --no-password --insecure --force \
    --not-after=876000h

# Generate server certificate
step certificate create "test-service2.test-ns.svc" server.pem server-key.pem \
    --ca server-ca.pem --ca-key server-ca-key.pem \
    --no-password --insecure --force \
    --not-after=876000h \
    --san "test-service2.test-ns.svc"

rm ./*.csr 2>/dev/null || true
