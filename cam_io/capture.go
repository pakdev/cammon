package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"log"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/pion/rtp"

	"github.com/cammon/cam_io/decoders"
)

type ImageMessage struct {
	Timestamp int64  `json:"timestamp"`
	ImageData []byte `json:"image_data"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type ImageCapture struct {
	rtspURL     string
	kafkaBroker string
	kafkaTopic  string

	producer *kafka.Producer
	client   *gortsplib.Client
	h264Dec  *decoders.H264Decoder
	url      *base.URL
}

func NewImageCapture(rtspURL string, kafkaBroker string, kafkaTopic string) *ImageCapture {
	return &ImageCapture{
		rtspURL:     rtspURL,
		kafkaBroker: kafkaBroker,
		kafkaTopic:  kafkaTopic,
	}
}

func (ic *ImageCapture) Initialize() error {
	var err error

	ic.producer, err = kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": ic.kafkaBroker})
	if err != nil {
		return err
	}

	ic.url, err = base.ParseURL(ic.rtspURL)
	if err != nil {
		return err
	}

	ic.client = &gortsplib.Client{
		Scheme: ic.url.Scheme,
		Host:   ic.url.Host,
	}

	err = ic.client.Start2()
	if err != nil {
		return err
	}

	ic.h264Dec = &decoders.H264Decoder{}
	err = ic.h264Dec.Initialize()
	if err != nil {
		return err
	}

	return nil
}

func (ic *ImageCapture) Run(ctx context.Context) error {
	desc, _, err := ic.client.Describe(ic.url)
	if err != nil {
		return err
	}

	log.Printf("available medias: %v\n", desc.Medias)

	var format *format.H264
	media := desc.FindFormat(&format)
	if media == nil {
		log.Fatal("media not found")
	}

	rtpDec, err := format.CreateDecoder()
	if err != nil {
		return err
	}

	_, err = ic.client.Setup(desc.BaseURL, media, 0, 0)
	if err != nil {
		return err
	}

	ic.client.OnPacketRTP(media, format, func(pkt *rtp.Packet) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		au, err := rtpDec.Decode(pkt)
		if err != nil {
			log.Printf("decode error: %v", err)
			return
		}

		img, err := ic.h264Dec.Decode(au)
		if err != nil {
			log.Fatal(err)
		}

		if img == nil {
			log.Printf("ERR: frame cannot be decoded")
			return
		}

		err = ic.sendImageToKafka(img)
		if err != nil {
			log.Printf("Failed to send image to Kafka: %v", err)
			return
		}
	})

	_, err = ic.client.Play(nil)
	if err != nil {
		return err
	}

	panic(ic.client.Wait())
}

func (ic *ImageCapture) Stop() {
	if ic.h264Dec != nil {
		ic.h264Dec.Close()
	}

	if ic.client != nil {
		ic.client.Close()
	}

	if ic.producer != nil {
		ic.producer.Close()
	}
}

func (ic *ImageCapture) sendImageToKafka(img image.Image) error {
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 60})
	if err != nil {
		return err
	}

	bounds := img.Bounds()
	message := ImageMessage{
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
		ImageData: buf.Bytes(),
		Width:     bounds.Dx(),
		Height:    bounds.Dy(),
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}

	err = ic.producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &ic.kafkaTopic, Partition: kafka.PartitionAny},
		Value:          messageBytes,
	}, nil)

	if err != nil {
		return err
	}

	log.Printf("Sent image to Kafka topic %s (size: %dx%d, %d bytes)", ic.kafkaTopic, bounds.Dx(), bounds.Dy(), len(buf.Bytes()))
	return nil
}