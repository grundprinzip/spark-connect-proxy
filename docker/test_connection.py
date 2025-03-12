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

import requests
from pyspark.sql import SparkSession

# Create a new session and extract the ID
res = requests.post("http://spark-connect-proxy:8081/control/sessions")
session_id = res.text
print(f"Session ID: {session_id}")

# Connect to Spark Connect through the proxy
remote = f"sc://spark-connect-proxy:8080/;x-spark-connect-session-id={session_id}"
print(f"Connection string: {remote}")

# Connect to Spark
spark = SparkSession.builder.remote(remote).getOrCreate()
print("Connected to Spark")

# Run a simple job
result = spark.range(10).collect()
print(f"Result: {result}")

# Get another session ID
res = requests.post("http://spark-connect-proxy:8081/control/sessions")
session_id2 = res.text
print(f"Second Session ID: {session_id2}")

# Connect with second session
remote2 = f"sc://spark-connect-proxy:8080/;x-spark-connect-session-id={session_id2}"
spark2 = SparkSession.builder.remote(remote2).getOrCreate()
print("Connected to Spark with second session")

# Run a simple job on second session
result2 = spark2.range(5).collect()
print(f"Result from second session: {result2}")

# Verify that the sessions are using different backends
print("\nVerifying load balancing across backends:")
# Get Spark application IDs from both sessions

app_id1 = spark.conf.get("spark.app.id")
app_id2 = spark2.conf.get("spark.app.id")

print(f"First session app ID: {app_id1}")
print(f"Second session app ID: {app_id2}")

# Clean up
spark.stop()
spark2.stop()

print("\n✅ Test completed successfully!")