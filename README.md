# Spark Connect Proxy

[![Docker Compose Test](https://github.com/grundprinzip/spark-connect-proxy/actions/workflows/docker-compose-test.yml/badge.svg)](https://github.com/grundprinzip/spark-connect-proxy/actions/workflows/docker-compose-test.yml)

When using Spark Connect as part of Apache Spark it is possible to
seamlessly connect to the Spark Cluster from PySpark directly without
running co-located to the driver.

However, Apache Spark does not provide to multiplex multiple users
across multiple Spark Clusters. This is where Spark Connect Proxy comes
in. It is a simple proxy server that can be used to multiplex multiple
users across multiple Spark Clusters.

It operates in spirit similar to other projects like Apache Livy or 
Apache Kyuubi.

## Installation

To install Spark Connect Proxy simply checkout this repository and run

```bash
make
```

Alternatively, you can download the latest binaries from the release page.
The binaries are pre-compiled for Linux, Apple, and Windows for both X86 and ARM
architectures.

Now you can start the server by running

```bash
./cmd/spark-connect-proxy/spark-connect-proxy
```

### Docker Setup
If you want to see how to setup the Spark Connect Proxy in a multi-backend scenario,
please have a look at the example using Docker Compose. This setup includes:
- Spark Connect Proxy
- Two Spark instances with Spark Connect enabled
- Automatic testing container

See the [Docker Compose Setup](README.docker.md) for additional documentation.

## Configuration

The proxy server can be configured using a YAML file. The following
example shows how to configure the proxy server to connect to a
pre-defined Spark cluster.

```yaml
---
backend_provider:
  # This is an arbitrary name to identify the backend provider.
  name: manual spark
  # Configures a pre-defined backend type that provides a list of already
  # started Spark clusters.
  type: PREDEFINED
  spec:
    endpoints:
      # A list of endpoints that the proxy can connect to.
      - url: localhost:15002
# Log level to use by the proxy.
log_level: debug
```

## Usage

Please check out the following video:

![Spark Connect Proxy](./docs/spark-connect-proxy.gif)

To try out the proxy server you can use the following example setup:

### Start Spark with Spark Connect

```shell
env SPARK_NO_DAEMONIZE=1 ./sbin/start-connect-server.sh --conf spark.log.structuredLogging.enabled=false --packages org.apache.spark:spark-connect_2.12:3.5.4
```

### Start Spark Connect Proxy

```shell
./cmd/spark-connect-proxy/spark-connect-proxy
```

### Connect to the Proxy to Connect to Spark

```python
import requests
from pyspark.sql import SparkSession

# Create a new session and extract the ID
res = requests.post("http://localhost:8081/control/sessions")
id = res.text

# Connect to Spark Connect on port 8080 which is the default
# port for the proxy, Spark Connect usually listens on 15002.
remote = f"sc://localhost:8080/;x-spark-connect-session-id={id}"

# Connect to Spark
spark = SparkSession.builder.remote(remote).getOrCreate()
spark.range(10).collect()
```

## Extending the Proxy with Custom Backend Providers

TODO

## Help Needed
It would be great to further extend this project and make it more useful. For
example there are still a lot of different topics that would be great to cover:

- [ ] Add support for more backend providers
- [ ] Add support for authentication and authorization as GRPC middleware
- [ ] And many others ...

Please reach out or create a pull request!