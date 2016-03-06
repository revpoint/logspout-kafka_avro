package kafka_avro

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	avro "github.com/elodina/go-avro"
	kafkaavro "github.com/elodina/go-kafka-avro"
	"github.com/gliderlabs/logspout/router"
	"gopkg.in/Shopify/sarama.v1"
)

func init() {
	router.AdapterFactories.Register(NewKafkaAvroAdapter, "kafka_avro")
}

type KafkaAvroAdapter struct {
	route       *router.Route
	brokers     []string
	topic       string
	schema_url  string
	registry    kafkaavro.KafkaAvroEncoder
	producer    sarama.AsyncProducer
	tmpl        *template.Template
}

func NewKafkaAvroAdapter(route *router.Route) (router.LogAdapter, error) {
	brokers := readBrokers(route.Address)
	if len(brokers) == 0 {
		return nil, errorf("The Kafka broker host:port is missing. Did you specify it as a route address?")
	}

	topic := readTopic(route.Address, route.Options)
	if topic == "" {
		return nil, errorf("The Kafka topic is missing. Did you specify it as a route option?")
	}

	schemaUrl := readSchemaRegistryUrl(route.Options)
	if schemaUrl == "" {
		return nil, errorf("The schema registry url is missing. Did you specify it as a route option?")
	}
	registry := kafkaavro.NewKafkaAvroEncoder(schemaUrl)

	var err error
	var tmpl *template.Template
	if rawTmpl := os.Getenv("KAFKA_TEMPLATE"); rawTmpl != "" {
		tmpl, err = template.New("kafka").Parse(rawTmpl)
		if err != nil {
			return nil, errorf("Couldn't parse Kafka message template. %v", err)
		}
	}

	var schema *avro.Schema
	if rawSchema := os.Getenv("KAFKA_AVRO_SCHEMA"); rawSchema != "" {
		schema, err := avro.ParseSchema(rawSchema)
		if err != nil {
			return nil, errorf("Couldn't parse Avro schema. %v", err)
		}
		if tmpl == nil {
			return nil, errorf("You must set KAFKA_TEMPLATE if you use a schema %v", err)
		}
	}

	if os.Getenv("DEBUG") != "" {
		log.Printf("Starting Kafka producer for address: %s, topic: %s.\n", brokers, topic)
	}

	var retries int
	retries, err = strconv.Atoi(os.Getenv("KAFKA_CONNECT_RETRIES"))
	if err != nil {
		retries = 3
	}
	var producer sarama.AsyncProducer
	for i := 0; i < retries; i++ {
		producer, err = sarama.NewAsyncProducer(brokers, newConfig())
		if err != nil {
			if os.Getenv("DEBUG") != "" {
				log.Println("Couldn't create Kafka producer. Retrying...", err)
			}
			if i == retries-1 {
				return nil, errorf("Couldn't create Kafka producer. %v", err)
			}
		} else {
			time.Sleep(1 * time.Second)
		}
	}

	return &KafkaAvroAdapter{
		route:     route,
		brokers:   brokers,
		topic:     topic,
		schemaUrl: schemaUrl,
		producer:  producer,
		tmpl:      tmpl,
		schema:    schema,
	}, nil
}

func (a *KafkaAvroAdapter) Stream(logstream chan *router.Message) {
	defer a.producer.Close()
	for rm := range logstream {
		message, err := a.formatMessage(rm)
		if err != nil {
			log.Println("kafka:", err)
			a.route.Close()
			break
		}

		a.producer.Input() <- message
	}
}

func newConfig() *sarama.Config {
	config := sarama.NewConfig()
	config.ClientID = "logspout"
	config.Producer.Return.Errors = false
	config.Producer.Return.Successes = false
	config.Producer.Flush.Frequency = 1 * time.Second
	config.Producer.RequiredAcks = sarama.WaitForLocal

	if opt := os.Getenv("KAFKA_COMPRESSION_CODEC"); opt != "" {
		switch opt {
		case "gzip":
			config.Producer.Compression = sarama.CompressionGZIP
		case "snappy":
			config.Producer.Compression = sarama.CompressionSnappy
		}
	}

	return config
}

func (a *KafkaAvroAdapter) formatMessage(message *router.Message) (*sarama.ProducerMessage, error) {
	var encoder sarama.Encoder
	if a.tmpl != nil {
		var w bytes.Buffer
		if err := a.tmpl.Execute(&w, message); err != nil {
			return nil, err
		}
		if a.schema != nil {
			var data interface{}
			if err := json.Unmarshal([]w.Bytes(), &data); err != nil {
				data = w.Bytes()
			}
		}
		encoder = sarama.ByteEncoder(w.Bytes())
	} else {
		encoder = sarama.StringEncoder(message.Data)
	}

	return &sarama.ProducerMessage{
		Topic: a.topic,
		Value: encoder,
	}, nil
}

func readBrokers(address string) []string {
	if strings.Contains(address, "/") {
		slash := strings.Index(address, "/")
		address = address[:slash]
	}

	return strings.Split(address, ",")
}

func readTopic(address string, options map[string]string) string {
	var topic string
	if !strings.Contains(address, "/") {
		topic = options["topic"]
	} else {
		slash := strings.Index(address, "/")
		topic = address[slash+1:]
	}

	return topic
}

func readSchemaRegistryUrl(options map[string]string) string {
	var url string
	if _, ok := options["schema_registry_url"]; ok {
		url = options["schema_registry_url"]
	}
	return url
}

func errorf(format string, a ...interface{}) (err error) {
	err = fmt.Errorf(format, a...)
	if os.Getenv("DEBUG") != "" {
		fmt.Println(err.Error())
	}
	return
}
