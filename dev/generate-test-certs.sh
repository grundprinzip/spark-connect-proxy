#!/bin/bash
# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#	http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script generates a self-signed certificate for testing TLS with the Spark Connect Proxy

set -e

CERT_DIR="certs"
mkdir -p "$CERT_DIR"

# Generate CA key and certificate
echo "Generating CA key and certificate..."
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout "$CERT_DIR/ca.key" \
  -out "$CERT_DIR/ca.crt" \
  -subj "/CN=Spark Connect Proxy CA"

# Generate server private key
echo "Generating server private key..."
openssl genrsa -out "$CERT_DIR/server.key" 2048

# Create server certificate signing request (CSR)
echo "Creating server certificate signing request..."
openssl req -new \
  -key "$CERT_DIR/server.key" \
  -out "$CERT_DIR/server.csr" \
  -subj "/CN=localhost"

# Create server certificate configuration with SAN (Subject Alternative Name)
cat > "$CERT_DIR/server.ext" << EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
IP.1 = 127.0.0.1
EOF

# Sign the server CSR with the CA certificate
echo "Signing server certificate with CA..."
openssl x509 -req -days 365 \
  -in "$CERT_DIR/server.csr" \
  -CA "$CERT_DIR/ca.crt" \
  -CAkey "$CERT_DIR/ca.key" \
  -CAcreateserial \
  -out "$CERT_DIR/server.crt" \
  -extfile "$CERT_DIR/server.ext"

# Clean up temporary files
rm "$CERT_DIR/server.csr" "$CERT_DIR/server.ext" "$CERT_DIR/ca.srl"

echo "Self-signed certificates generated successfully in $CERT_DIR/"
echo "To enable TLS in your config, set:"
echo "server.tls.enabled: true"
echo "server.tls.cert_file: \"$CERT_DIR/server.crt\""
echo "server.tls.key_file: \"$CERT_DIR/server.key\""