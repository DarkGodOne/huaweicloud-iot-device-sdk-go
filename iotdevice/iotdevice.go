package iotdevice

import (
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"huaweicloud-iot-device-sdk-go/handlers"
	"strings"
)

type IotDevice interface {
	Init() bool
	IsConnected() bool
	SendMessage(message handlers.IotMessage) bool
	ReportProperties(properties handlers.IotServiceProperty) bool
	AddMessageHandler(handler handlers.IotMessageHandler)
	AddCommandHandler(handler handlers.IotCommandHandler)
	AddPropertiesSetHandler(handler handlers.IotDevicePropertiesSetHandler)
	SetPropertyQueryHandler(handler handlers.IotDevicePropertyQueryHandler)
}

type iotDevice struct {
	Id                    string
	Password              string
	Servers               string
	client                mqtt.Client
	commandHandlers       []handlers.IotCommandHandler
	messageHandlers       []handlers.IotMessageHandler
	propertiesSetHandlers []handlers.IotDevicePropertiesSetHandler
	propertyQueryHandler  handlers.IotDevicePropertyQueryHandler
	topics                map[string]string
}

func (device *iotDevice) createMessageMqttHandler() func(client mqtt.Client, message mqtt.Message) {
	messageHandler := func(client mqtt.Client, message mqtt.Message) {
		msg := &handlers.IotMessage{}
		if json.Unmarshal(message.Payload(), msg) != nil {
			fmt.Println("unmarshal device message failed")
		}

		for _, handler := range device.messageHandlers {
			handler(*msg)
		}
	}

	return messageHandler
}

func (device *iotDevice) createCommandMqttHandler() func(client mqtt.Client, message mqtt.Message) {
	commandHandler := func(client mqtt.Client, message mqtt.Message) {
		command := &handlers.IotCommand{}
		if json.Unmarshal(message.Payload(), command) != nil {
			fmt.Println("unmarshal failed")
		}

		handleFlag := true
		for _, handler := range device.commandHandlers {
			handleFlag = handleFlag && handler(*command)
		}
		var res string
		if handleFlag {
			res = JsonString(handlers.SuccessIotCommandResponse())
		} else {
			res = JsonString(handlers.FailedIotCommandResponse())
		}
		if token := device.client.Publish(device.topics[CommandResponseTopicName]+TopicRequestId(message.Topic()), 1, false, res);
			token.Wait() && token.Error() != nil {
			fmt.Println("send command response failed")
		}
	}

	return commandHandler
}

func (device *iotDevice) createPropertiesSetMqttHandler() func(client mqtt.Client, message mqtt.Message) {
	propertiesSetHandler := func(client mqtt.Client, message mqtt.Message) {
		propertiesSetRequest := &handlers.IotDevicePropertyDownRequest{}
		if json.Unmarshal(message.Payload(), propertiesSetRequest) != nil {
			fmt.Println("unmarshal failed")
		}

		handleFlag := true
		for _, handler := range device.propertiesSetHandlers {
			handleFlag = handleFlag && handler(*propertiesSetRequest)
		}

		var res string
		if handleFlag {
			res = JsonString(handlers.SuccessPropertiesSetResponse())
		} else {
			res = JsonString(handlers.FailedPropertiesSetResponse())
		}
		if token := device.client.Publish(device.topics[PropertiesSetResponseTopicName]+TopicRequestId(message.Topic()), 1, false, res);
			token.Wait() && token.Error() != nil {
			fmt.Println("send properties set response failed")
		}
	}

	return propertiesSetHandler
}

func (device *iotDevice) createPropertiesQueryMqttHandler() func(client mqtt.Client, message mqtt.Message) {
	propertiesQueryHandler := func(client mqtt.Client, message mqtt.Message) {
		propertiesQueryRequest := &handlers.IotDevicePropertyQueryRequest{}
		if json.Unmarshal(message.Payload(), propertiesQueryRequest) != nil {
			fmt.Println("unmarshal failed")
		}

		queryResult := device.propertyQueryHandler(*propertiesQueryRequest)
		responseToPlatform := JsonString(queryResult)
		if token := device.client.Publish(device.topics[PropertiesQueryResponseTopicName]+TopicRequestId(message.Topic()), 1, false, responseToPlatform);
			token.Wait() && token.Error() != nil {
			fmt.Println("send properties set response failed")
		}
	}

	return propertiesQueryHandler
}

func (device *iotDevice) Init() bool {
	options := mqtt.NewClientOptions()
	options.AddBroker(device.Servers)
	options.SetClientID(assembleClientId(device))
	options.SetUsername(device.Id)
	options.SetPassword(HmacSha256(device.Password, TimeStamp()))

	device.client = mqtt.NewClient(options)

	if token := device.client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Printf("IoT device init failed,caulse %s\n", token.Error())
		return false
	}

	device.subscribeDefaultTopics()

	return true

}

func (device *iotDevice) subscribeDefaultTopics() {
	// 订阅平台命令下发topic
	if token := device.client.Subscribe(device.topics[CommandDownTopicName], 2, device.createCommandMqttHandler());
		token.Wait() && token.Error() != nil {
		fmt.Println("subscribe command down topic failed")
		panic(0)
	}

	// 订阅平台消息下发的topic
	if token := device.client.Subscribe(device.topics[MessageDownTopicName], 2, device.createMessageMqttHandler());
		token.Wait() && token.Error() != nil {
		fmt.Println("subscribe message down topic failed")
		panic(0)
	}

	// 订阅平台设置设备属性的topic
	if token := device.client.Subscribe(device.topics[PropertiesSetRequestTopicName], 2, device.createPropertiesSetMqttHandler());
		token.Wait() && token.Error() != nil {
		fmt.Println("subscribe properties set topic failed")
		panic(0)
	}

	// 订阅平台查询设备属性的topic
	if token := device.client.Subscribe(device.topics[PropertiesQueryRequestTopicName], 2, device.createPropertiesQueryMqttHandler());
		token.Wait() && token.Error() != nil {
		fmt.Println("subscribe properties query topic failed")
		panic(0)
	}
}

func (device *iotDevice) IsConnected() bool {
	if device.client != nil {
		return device.client.IsConnected()
	}
	return false
}

func (device *iotDevice) SendMessage(message handlers.IotMessage) bool {
	messageData := JsonString(message)
	if token := device.client.Publish(device.topics[MessageUpTopicName], 2, false, messageData);
		token.Wait() && token.Error() != nil {
		fmt.Println("send message failed")
		return false
	}

	return true
}

func (device *iotDevice) ReportProperties(properties handlers.IotServiceProperty) bool {
	propertiesData := JsonString(properties)
	if token := device.client.Publish(device.topics[PropertiesUpTopicName], 2, false, propertiesData);
		token.Wait() && token.Error() != nil {
		fmt.Println("report properties failed")
		return false
	}
	return true
}

func (device *iotDevice) AddMessageHandler(handler handlers.IotMessageHandler) {
	if handler == nil {
		return
	}
	device.messageHandlers = append(device.messageHandlers, handler)
}

func (device *iotDevice) AddCommandHandler(handler handlers.IotCommandHandler) {
	if handler == nil {
		return
	}

	device.commandHandlers = append(device.commandHandlers, handler)
}

func (device *iotDevice) AddPropertiesSetHandler(handler handlers.IotDevicePropertiesSetHandler) {
	if handler == nil {
		return
	}
	device.propertiesSetHandlers = append(device.propertiesSetHandlers, handler)
}

func (device *iotDevice) SetPropertyQueryHandler(handler handlers.IotDevicePropertyQueryHandler) {
	device.propertyQueryHandler = handler
}

func assembleClientId(device *iotDevice) string {
	segments := make([]string, 4)
	segments[0] = device.Id
	segments[1] = "0"
	segments[2] = "0"
	segments[3] = TimeStamp()

	return strings.Join(segments, "_")
}

func CreateIotDevice(id, password, servers string) IotDevice {
	device := &iotDevice{}
	device.Id = id
	device.Password = password
	device.Servers = servers
	device.messageHandlers = []handlers.IotMessageHandler{}
	device.commandHandlers = []handlers.IotCommandHandler{}

	// 初始化设备相关的所有topic
	device.topics = make(map[string]string)
	device.topics[MessageDownTopicName] = TopicFormat(MessageDownTopic, id)
	device.topics[CommandDownTopicName] = TopicFormat(CommandDownTopic, id)
	device.topics[CommandResponseTopicName] = TopicFormat(CommandResponseTopic, id)
	device.topics[MessageUpTopicName] = TopicFormat(MessageUpTopic, id)
	device.topics[PropertiesUpTopicName] = TopicFormat(PropertiesUpTopic, id)
	device.topics[PropertiesSetRequestTopicName] = TopicFormat(PropertiesSetRequestTopic, id)
	device.topics[PropertiesSetResponseTopicName] = TopicFormat(PropertiesSetResponseTopic, id)
	device.topics[PropertiesQueryRequestTopicName] = TopicFormat(PropertiesQueryRequestTopic, id)
	device.topics[PropertiesQueryResponseTopicName] = TopicFormat(PropertiesQueryResponseTopic, id)
	return device
}
