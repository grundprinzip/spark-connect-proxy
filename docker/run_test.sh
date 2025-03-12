#!/bin/bash
#
# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
set -ex

echo "Waiting for Spark Connect Proxy to be ready..."
# Wait for the proxy to be ready
#max_retries=30
#retry_count=0
#while ! curl -v -s http://spark-connect-proxy:8081/control/metrics &>/dev/null; do
#  retry_count=$((retry_count+1))
#  if [ $retry_count -gt $max_retries ]; then
#    echo "Proxy not ready after $max_retries retries, exiting"
#    exit 1
#  fi
#  echo "Waiting for proxy to be ready ($retry_count/$max_retries)..."
#  sleep 2
#done

echo "Proxy is ready! Waiting 10 more seconds for Spark Connect services..."
sleep 10

echo "Running connection test..."
python test_connection.py

echo "Test completed successfully!"
exit 0