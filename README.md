# logspout-kafka_avro

A [Logspout](https://github.com/gliderlabs/logspout) adapter for writing Docker container logs to [Kafka](https://github.com/apache/kafka) topics which is encoded in [Avro](https://avro.apache.org) managed with [Confluent's Schema Registry](http://docs.confluent.io/2.0.0/schema-registry/docs/index.html).

This was originally based on [logspout-kafka](https://github.com/gettyimages/logspout-kafka) by Dylan Meissner.

## usage

With *container-logs* as the Kafka topic for Docker container logs, we can direct all messages to Kafka using the **logspout** [Route API](https://github.com/gliderlabs/logspout/tree/master/routesapi) while
saving the Avro schema with the Schema Registry. In order to avoid errors at runtime, make sure that
you set the subject compatibility to `NONE`:

```
curl http://container-host:8000/routes -d '{
  "adapter": "kafka_avro",
  "filter_sources": ["stdout" ,"stderr"],
  "address": "kafka-broker1:9092,kafka-broker2:9092/container-logs",
  "options": {
    "schema_registry_url": "schema-registry:8081"
  }
}'
```

## message template configuration

The default behavior is to route the raw message to Kafka.  This adapter provides a mechanism for customizing the template to include additional metadata via the `KAFKA_TEMPLATE` environment variable.  Golang's [text/template](http://golang.org/pkg/text/template/) package is used for templating, where the model available for templating is the [Message struct](https://github.com/gliderlabs/logspout/blob/master/router/types.go).

The following example `KAFKA_TEMPLATE` configuration appends additional metadata to each log message received:
```
KAFKA_TEMPLATE="{\"timestamp\": \"{{.Time}}\", \"container_name\": \"{{.Container.Name}}\", \"source\": \"{{.Source}}\", \"line\": \"{{.Data}}\"}"
```

Example input and output
```
Hello World

{"timestamp": "2015-06-23 09:54:55.241951004 +0000 UTC", "container_name": "/hello_container", "source": "stdout", "line": "Hello World"}
```

## avro schema configuration

The default behavior is to a primitive string schema (`{"type" : "%s" }`).  This adapter provides a mechanism for customizing the Avro schema via the `KAFKA_AVRO_SCHEMA` environment variable. This schema should match the `KAFKA_TEMPLATE` if you set `KAFKA_AVRO_SCHEMA`.

The following example `KAFKA_AVRO_SCHEMA` configuration matches the template above:
```
KAFKA_AVRO_SCHEMA="{\"type\":\"record\",\"name\":\"logLine\",\"fields\":[{\"name\":\"timestamp\",\"type\":\"string\"},{\"name\":\"container_name\",\"type\":\"string\"},{\"name\":\"source\",\"type\":\"string\"},{\"name\":\"line\",\"type\":\"string\"}]}"
```

## route configuration

If you've mounted a volume to `/mnt/routes`, then consider pre-populating your routes. The following script configures a route to send standard messages from a "cat" container to one Kafka topic, and a route to send standard/error messages from a "dog" container to another topic.

```
cat > /logspout/routes/cat.json <<CAT
{
  "id": "cat",
  "adapter": "kafka_avro",
  "filter_name": "cat_*",
  "filter_sources": ["stdout"],
  "address": "kafka-broker1:9092,kafka-broker2:9092/cat-logs",
  "options": {
    "schema_registry_url": "schema-registry:8081"
  }
}
CAT

cat > /logspout/routes/dog.json <<DOG
{
  "id": "dog",
  "adapter": "kafka_avro",
  "filter_name": "dog_*",
  "filter_sources": ["stdout", "stderr"],
  "address": "kafka-broker1:9092,kafka-broker2:9092/dog-logs",
  "options": {
    "schema_registry_url": "schema-registry:8081"
  }
}
DOG

docker run --name logspout \
  -p "8000:8000" \
  --volume /logspout/routes:/mnt/routes \
  --volume /var/run/docker.sock:/tmp/docker.sock \
  revpoint/logspout-kafka_avro

```

The routes can be updated on a running container by using the **logspout** [Route API](https://github.com/gliderlabs/logspout/tree/master/routesapi) and specifying the route `id` "cat" or "dog".

## build

**logspout-kafka** is a custom **logspout** module. To use it, create an empty `Dockerfile` based on `gliderlabs/logspout` and include this **logspout-kafka** module in a new `modules.go` file.

The following example creates an almost-minimal **logspout** image capable of writing Docker container logs to Kafka topics:

```
cat > ./Dockerfile.example <<DOCKERFILE
FROM gliderlabs/logspout:master

ENV KAFKA_COMPRESSION_CODEC snappy
DOCKERFILE

cat > ./modules.go <<MODULES
package main
import (
  _ "github.com/gliderlabs/logspout/httpstream"
  _ "github.com/gliderlabs/logspout/routesapi"
  _ "github.com/gettyimages/logspout-kafka_avro"
)
MODULES

docker build -t gettyimages/example-logspout -f Dockerfile.example .
```

More info about building custom modules is available at the **logspout** project: [Custom Logspout Modules](https://github.com/gliderlabs/logspout/blob/master/custom/README.md)
