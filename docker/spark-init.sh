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
set -e

SPARK_CONNECT_PORT=$1
SPARK_UI_PORT=$2

# Create a directory for logs
mkdir -p /tmp/spark-logs

# Start Spark Connect Server
exec /opt/bitnami/spark/bin/spark-submit \
  --class org.apache.spark.sql.connect.service.SparkConnectServer \
  --packages org.apache.spark:spark-connect_2.12:3.5.5 \
  --conf spark.connect.grpc.binding.port=${SPARK_CONNECT_PORT} \
  --conf spark.connect.grpc.binding.host=0.0.0.0 \
  --conf spark.ui.port=${SPARK_UI_PORT} \
  --conf spark.driver.host=$(hostname -i) \
  --conf spark.driver.bindAddress=0.0.0.0 \
  --conf spark.log.structuredLogging.enabled=false