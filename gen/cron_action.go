package gen

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

const (
	CronEnvNodeName      = Env("CRON_NODE_NAME")
	CronEnvJobName       = Env("CRON_JOB_NAME")
	CronEnvJobActionTime = Env("CRON_JOB_ACTION_TIME")
)

type CronAction interface {
	Do(job Atom, node Node, atime time.Time) error
	Info() string
}

func CreateCronActionMessage[To Atom | ProcessID | PID | Alias](to To, priority MessagePriority) CronAction {
	return &cronActionMessage{
		to:       to,
		priority: priority,
		info:     fmt.Sprintf("send message to %s with %s priority", to, priority),
	}
}

// action send message

type cronActionMessage struct {
	to       any
	priority MessagePriority
	info     string
}

func (cam *cronActionMessage) Do(job Atom, node Node, atime time.Time) error {
	message := MessageCron{
		Node: node.Name(),
		Job:  job,
		Time: atime,
	}
	return node.SendWithPriority(cam.to, message, cam.priority)
}

func (cam *cronActionMessage) Info() string {
	return cam.info
}

// action Spawn

type cronActionSpawn struct {
	factory ProcessFactory
	options CronActionSpawnOptions
	info    string
}

type CronActionSpawnOptions struct {
	Register       Atom
	ProcessOptions ProcessOptions
	Args           []any
}

func CreateCronActionSpawn(factory ProcessFactory, options CronActionSpawnOptions) CronAction {
	if factory == nil {
		panic("nil value as ProcessFactory")
	}

	if options.ProcessOptions.Env == nil {
		options.ProcessOptions.Env = make(map[Env]any)
	}

	cas := &cronActionSpawn{
		factory: factory,
		options: options,
	}

	behavior := strings.TrimPrefix(reflect.TypeOf(factory()).String(), "*")
	if options.Register == "" {
		cas.info = fmt.Sprintf("spawn process using behavior %s", behavior)
		return cas
	}

	cas.info = fmt.Sprintf("spawn process with registered name %s using behavior %s",
		options.Register, behavior)
	return cas
}

func (cas *cronActionSpawn) Do(job Atom, node Node, atime time.Time) error {
	processOptions := cas.options.ProcessOptions

	// clone env
	processOptions.Env = make(map[Env]any)
	for k, v := range cas.options.ProcessOptions.Env {
		processOptions.Env[k] = v
	}
	processOptions.Env[CronEnvNodeName] = node.Name()
	processOptions.Env[CronEnvJobName] = job
	processOptions.Env[CronEnvJobActionTime] = atime

	if cas.options.Register == "" {
		_, err := node.Spawn(cas.factory, processOptions,
			cas.options.Args...)
		return err
	}
	_, err := node.SpawnRegister(cas.options.Register, cas.factory,
		processOptions, cas.options.Args...)
	return err
}

func (cas *cronActionSpawn) Info() string {
	return cas.info
}

// action remote Spawn

func CreateCronActionRemoteSpawn(node Atom, name Atom, options CronActionSpawnOptions) CronAction {
	cars := &cronActionRemoteSpawn{
		node:    node,
		name:    name,
		options: options,
	}
	if options.Register == "" {
		cars.info = fmt.Sprintf("spawn remote process on %s using process factory %s",
			node, name)
		return cars
	}

	cars.info = fmt.Sprintf("spawn remote process with registered name %s on %s using process factory %s",
		options.Register, node, name)
	return cars
}

type cronActionRemoteSpawn struct {
	node    Atom
	name    Atom
	options CronActionSpawnOptions
	info    string
}

func (cars *cronActionRemoteSpawn) Do(job Atom, node Node, atime time.Time) error {
	remote, err := node.Network().GetNode(cars.node)
	if err != nil {
		return err
	}

	processOptions := cars.options.ProcessOptions

	// clone env
	processOptions.Env = make(map[Env]any)
	for k, v := range cars.options.ProcessOptions.Env {
		processOptions.Env[k] = v
	}
	processOptions.Env[CronEnvNodeName] = node.Name()
	processOptions.Env[CronEnvJobName] = job
	processOptions.Env[CronEnvJobActionTime] = atime

	if cars.options.Register == "" {
		_, err := remote.Spawn(cars.name, processOptions,
			cars.options.Args...)
		return err
	}
	_, err = remote.SpawnRegister(cars.options.Register, cars.name,
		processOptions, cars.options.Args...)
	return err
}

func (cars *cronActionRemoteSpawn) Info() string {
	return ""
}
