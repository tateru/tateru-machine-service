package main

import (
	"errors"
	"sync"
)

type EventName string

type Event struct {
	NewState StateName
}

type EventMap map[EventName]Event

type StateName string

type State struct {
	Events EventMap
}

type StateMap map[StateName]State

type StateMachine struct {
	InitialState StateName
	States       StateMap

	current StateName
	cond    *sync.Cond
}

func NewStateMachine(initialState StateName, states StateMap) *StateMachine {
	return &StateMachine{
		InitialState: initialState,
		States:       states,
		cond:         sync.NewCond(&sync.Mutex{}),
	}
}

func (s *StateMachine) Current() StateName {
	s.cond.L.Lock()
	current := s.current
	s.cond.L.Unlock()

	if current == "" {
		current = s.InitialState
	}

	return current
}

func (s *StateMachine) String() string {
	return string(s.Current())
}

func (s *StateMachine) Transition(event EventName) (StateName, error) {
	current := s.Current()

	transitions := s.States[current].Events
	transition, ok := transitions[event]
	if !ok {
		return "", errors.New("Invalid transition: Event not available for current state")
	}

	if transition.NewState != "" {
		s.cond.L.Lock()
		s.current = transition.NewState
		s.cond.Broadcast()
		s.cond.L.Unlock()
	}

	return s.Current(), nil
}

func (s *StateMachine) WaitFor(state StateName) {
	s.cond.L.Lock()
	for s.current != state {
		s.cond.Wait()
	}
	s.cond.L.Unlock()
	return
}
