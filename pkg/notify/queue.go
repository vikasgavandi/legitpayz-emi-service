package notify

import (
	"context"
	"log"
	"os"

	"github.com/vikasgavandi/legitpayz-emi-service/pkg/awsx"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// Abstracts SQS + SNS. Floci/local: real SDK calls when AWS_ENDPOINT_URL is set.
// Otherwise: log stub.
type Queue interface {
	Enqueue(queueURL, body string) error
	PublishSMS(phone, msg string) error
}

type LogQueue struct{}

func (LogQueue) Enqueue(q, body string) error {
	log.Printf("[sqs] queue=%s body=%s", q, body)
	return nil
}
func (LogQueue) PublishSMS(phone, msg string) error {
	log.Printf("[sns] sms to=%s msg=%s", phone, msg)
	return nil
}

// FlociQueue sends to real SQS/SNS endpoints (Floci on :4566).
type FlociQueue struct{}

func queueURL(ctx context.Context, c *sqs.Client, nameOrURL string) (string, error) {
	if len(nameOrURL) > 4 && nameOrURL[:4] == "http" {
		return nameOrURL, nil
	}
	out, err := c.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{QueueName: aws.String(nameOrURL)})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.QueueUrl), nil
}

func (FlociQueue) Enqueue(nameOrURL, body string) error {
	ctx := context.Background()
	c, err := awsx.SQS(ctx)
	if err != nil {
		return err
	}
	u, err := queueURL(ctx, c, nameOrURL)
	if err != nil {
		return err
	}
	_, err = c.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: aws.String(u), MessageBody: aws.String(body)})
	if err == nil {
		log.Printf("[sqs:floci] sent queue=%s", u)
	}
	return err
}

func (FlociQueue) PublishSMS(phone, msg string) error {
	ctx := context.Background()
	c, err := awsx.SNS(ctx)
	if err != nil {
		return err
	}
	if phone != "" {
		_, err = c.Publish(ctx, &sns.PublishInput{PhoneNumber: aws.String(phone), Message: aws.String(msg)})
	} else {
		topic := os.Getenv("SNS_TOPIC_ARN")
		if topic == "" {
			topic = "arn:aws:sns:us-east-1:000000000000:cc-notifications"
		}
		_, err = c.Publish(ctx, &sns.PublishInput{TopicArn: aws.String(topic), Message: aws.String(msg)})
	}
	if err == nil {
		log.Printf("[sns:floci] published to=%s", phone)
	}
	return err
}

func resolve() Queue {
	if awsx.Enabled() {
		return FlociQueue{}
	}
	return LogQueue{}
}

var Default Queue = resolve()
