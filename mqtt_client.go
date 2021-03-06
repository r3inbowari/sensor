package sensor

import (
	"fmt"
	"github.com/eclipse/paho.mqtt.golang"
)

// ws/ssl/tcp
// var scheme = "tcp"
// var host = "106.13.79.157"
// var port = "1883"

// ClientID 随机acm0-bjd2-fdi1-am81
// var ClientID = bson.NewObjectId().String()
// var Username = "r3inb"
// var Password = "159463"

var defaultPublishHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	// drop
}

var client mqtt.Client = nil

func GetMQTTInstance() (mqtt.Client, error) {
	if client == nil || !client.IsConnectionOpen() {
		if ins, err := pMQTTClient(); err != nil {
			return nil, err
		} else {
			client = ins
			fmt.Println("[CONN] 已连接到MQ: " + GetBrokerIP())
		}
	}
	return client, nil
}

func pMQTTClient() (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(GetBrokerScheme() + "://" + GetBrokerIP() + ":" + GetBrokerPort())
	// MQ ClientID
	opts.SetClientID(GetBrokerClientID())
	// MQ 账号/密码
	opts.SetUsername(GetBrokerUsername())
	opts.SetPassword(GetBrokerPassword())
	// opts.SetKeepAlive(2 * time.Second)
	// 默认消费方式
	//opts.SetDefaultPublishHandler(defaultPublishHandler)
	// ping超时
	//opts.SetPingTimeout(1 * time.Second)

	c := mqtt.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		fmt.Println("[FAIL] MQTT Broker connect failed")
		return nil, token.Error()
	}
	return c, nil
}

func MQTTMapping(topic string, callback mqtt.MessageHandler) bool {
	if mq, err := GetMQTTInstance(); err != nil {
		return false
	} else {
		if token := mq.Subscribe(topic, 1, callback); token.Wait() && token.Error() != nil {
			fmt.Printf("subscribe failed by %s\n", topic)
			return false
		}
	}
	fmt.Printf("subscribed %s successfully\n", topic)
	return true
}

func MQTTPublish(topic string, payload interface{}) {
	if mq, err := GetMQTTInstance(); err != nil {
		fmt.Println("[FAIL] 发布失败")
	} else {
		token := mq.Publish(topic, 1, false, payload)
		token.Wait()
	}
}
