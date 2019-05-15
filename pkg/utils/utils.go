package utils

import (
	"time"
	"math/rand"

	"k8s.io/klog"
)

const letters = "abcdefghijklmnopqrstuvwxyz0123456789"

func RandString(n int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

type NamedLog struct {
	Name string
}

func NewNamedLog(name string) *NamedLog {
	return &NamedLog{Name: name + " "}
}

func (b *NamedLog) Infof(format string, v ...interface{}) {
	klog.Infof(b.Name + format, v...)
}

func (b *NamedLog) Info(string string) {
	klog.Info(b.Name + string)
}
