// Package events abstracts Kafka. In local dev it logs; in AWS MSK/Kafka it publishes.
// Topic conventions: customer.events, card.events, txn.events, emi.events
package events

import "log"

type Publisher interface {
	Publish(topic, key, payload string) error
}

type LogPublisher struct{}

func (LogPublisher) Publish(topic, key, payload string) error {
	log.Printf("[kafka] topic=%s key=%s payload=%s", topic, key, payload)
	return nil
}

var Default Publisher = LogPublisher{}
