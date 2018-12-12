package mq

import (
	"reflect"
	"strings"
	"time"

	"eventter.io/mq/client"
)

func messageMatches(message *client.Message, messageTime time.Time, topic *ClusterTopic, consumerGroup *ClusterConsumerGroup) bool {
	if messageTime.Before(consumerGroup.CreatedAt) {
		return false
	}

	switch topic.Type {
	case client.TopicType_DIRECT:
		for _, binding := range consumerGroup.Bindings {
			if binding.TopicName != topic.Name {
				continue
			}

			if by, ok := binding.By.(*ClusterConsumerGroup_Binding_RoutingKey); ok && by.RoutingKey == message.RoutingKey {
				return true
			}
		}
		return false

	case client.TopicType_FANOUT:
		return true

	case client.TopicType_TOPIC:
		for _, binding := range consumerGroup.Bindings {
			if binding.TopicName != topic.Name {
				continue
			}

			if by, ok := binding.By.(*ClusterConsumerGroup_Binding_RoutingKey); ok && routingKeyMatches(by.RoutingKey, message.RoutingKey) {
				return true
			}
		}
		return false

	case client.TopicType_HEADERS:
		if message.Headers == nil || message.Headers.Fields == nil {
			return false
		}
	BINDING:
		for _, binding := range consumerGroup.Bindings {
			if binding.TopicName != topic.Name {
				continue
			}

			switch by := binding.By.(type) {
			case *ClusterConsumerGroup_Binding_HeadersAll:
				for headerName, expectedHeaderValue := range by.HeadersAll.Fields {
					gotHeaderValue, ok := message.Headers.Fields[headerName]
					if !ok {
						continue BINDING
					}
					if !reflect.DeepEqual(expectedHeaderValue, gotHeaderValue) {
						continue BINDING
					}
				}
				return true
			case *ClusterConsumerGroup_Binding_HeadersAny:
				for headerName, expectedHeaderValue := range by.HeadersAny.Fields {
					gotHeaderValue, ok := message.Headers.Fields[headerName]
					if !ok {
						continue
					}
					if reflect.DeepEqual(expectedHeaderValue, gotHeaderValue) {
						return true
					}
				}
				return false
			}
		}
		return false

	default:
		panic("unhandled topic type: " + topic.Type)
	}
}

const (
	patternSeparator  = '.'
	patternWildcard   = "*"
	patternZeroOrMore = "#"
)

func routingKeyMatches(pattern, routingKey string) (ret bool) {
	if pattern == "" {
		return routingKey == ""
	}

	i := strings.IndexByte(pattern, patternSeparator)
	if i == -1 {
		if pattern == patternWildcard {
			return routingKey != "" && strings.IndexByte(routingKey, patternSeparator) == -1
		} else if pattern == patternZeroOrMore {
			j := strings.IndexByte(routingKey, patternSeparator)
			if j == -1 {
				return true
			}
			routingKey = routingKey[j+1:]
			if routingKey == "" { // trailing dot
				return false
			}
			return routingKeyMatches(pattern, routingKey)
		} else {
			return routingKey == pattern
		}
	} else {
		part := pattern[:i]
		rest := pattern[i+1:]

		if part == patternWildcard {
			j := strings.IndexByte(routingKey, patternSeparator)
			if j == -1 {
				if routingKey == "" {
					return false
				}
				return routingKeyMatches(rest, "")
			}
			routingKey = routingKey[j+1:]
			if routingKey == "" { // trailing dot
				return false
			}
			return routingKeyMatches(rest, routingKey)

		} else if part == patternZeroOrMore {
			for {
				if routingKeyMatches(rest, routingKey) {
					return true
				}
				j := strings.IndexByte(routingKey, patternSeparator)
				if j == -1 {
					return false
				}
				routingKey = routingKey[j+1:]
				if routingKey == "" { // trailing dot
					return false
				}
			}

		} else {
			j := strings.IndexByte(routingKey, patternSeparator)
			if j == -1 {
				return routingKey == part && routingKeyMatches(rest, "")
			}
			if routingKey[:j] != part {
				return false
			}
			routingKey = routingKey[j+1:]
			if routingKey == "" { // trailing dot
				return false
			}
			return routingKeyMatches(rest, routingKey)
		}
	}
}