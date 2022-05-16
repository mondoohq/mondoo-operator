package nodes

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCronJobName(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	prefix := "mondoo-client"
	tests := []struct {
		name string
		data func() (suffix, expected string)
	}{
		{
			name: "should be prefix+base+suffix when shorter than 52 chars",
			data: func() (suffix, expected string) {
				base := fmt.Sprintf("%s%s", prefix, CronJobNameBase)
				suffix = RandString(52 - len(base))
				return suffix, fmt.Sprintf("%s%s", base, suffix)
			},
		},
		{
			name: "should be prefix+base+hash when longer than 52 chars",
			data: func() (suffix, expected string) {
				base := fmt.Sprintf("%s%s", prefix, CronJobNameBase)
				suffix = RandString(53 - len(base))

				hash := fmt.Sprintf("%x", sha256.Sum256([]byte(suffix)))
				return suffix, fmt.Sprintf("%s%s", base, hash[:52-len(base)])
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			suffix, expected := test.data()
			assert.Equal(t, expected, CronJobName(prefix, suffix))
		})
	}
}

func TestConfigMapName(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	prefix := "mondoo-client"
	tests := []struct {
		name string
		data func() (suffix, expected string)
	}{
		{
			name: "should be prefix+base+suffix when shorter than 52 chars",
			data: func() (suffix, expected string) {
				base := fmt.Sprintf("%s%s", prefix, InventoryConfigMapBase)
				suffix = RandString(52 - len(base))
				return suffix, fmt.Sprintf("%s%s", base, suffix)
			},
		},
		{
			name: "should be prefix+base+hash when longer than 52 chars",
			data: func() (suffix, expected string) {
				base := fmt.Sprintf("%s%s", prefix, InventoryConfigMapBase)
				suffix = RandString(53 - len(base))

				hash := fmt.Sprintf("%x", sha256.Sum256([]byte(suffix)))
				return suffix, fmt.Sprintf("%s%s", base, hash[:52-len(base)])
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			suffix, expected := test.data()
			assert.Equal(t, expected, ConfigMapName(prefix, suffix))
		})
	}
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}
