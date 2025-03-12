# Docker Compose Setup for Spark Connect Proxy

This setup includes four containers:
1. `spark-connect-proxy` - Runs the Spark Connect Proxy
2. `spark1` - Runs a Spark 3.5.0 instance with Spark Connect enabled on port 15002
3. `spark2` - Runs a second Spark 3.5.0 instance with Spark Connect enabled on port 15002 (mapped to 15003 on host)
4. `test` - A Python container that tests the setup by running sample queries through the proxy

## Prerequisites

- Docker and Docker Compose installed

## Usage

1. Start the Docker Compose setup:
   ```bash
   docker-compose up
   ```
   
   The test container will automatically run tests to verify that the setup works correctly.

2. Connect to the proxy using PySpark:
   ```python
   import requests
   from pyspark.sql import SparkSession

   # Create a new session and extract the ID
   res = requests.post("http://localhost:8081/control/sessions")
   id = res.text

   # Connect to Spark Connect through the proxy
   remote = f"sc://localhost:8080/;x-spark-connect-session-id={id}"

   # Connect to Spark
   spark = SparkSession.builder.remote(remote).getOrCreate()
   spark.range(10).collect()
   ```

3. Access Spark UIs:
   - Spark1 UI: http://localhost:4040
   - Spark2 UI: http://localhost:4041

4. Stop the Docker Compose setup:
   ```bash
   docker-compose down
   ```

## Architecture

This setup demonstrates a complete Spark Connect Proxy environment:

- The proxy container builds the binary from source code directly
- Two independent Spark clusters run in separate containers
- The proxy balances requests between the two Spark backends using round-robin load balancing
- The test container validates that the end-to-end setup works correctly

## Troubleshooting

If you encounter issues:

1. Check if the Spark Connect services are running:
   ```bash
   docker-compose logs spark1 spark2
   ```

2. Verify the proxy is properly connecting to the backends:
   ```bash
   docker-compose logs spark-connect-proxy
   ```

3. See test container logs to check connection issues:
   ```bash
   docker-compose logs test
   ```