package nwucp

import (
	"testing"

	"github.com/free5gc/nas/nasMessage"
	"github.com/free5gc/nas/nasType"
)

func TestConfigurationUpdateNeedsAck(t *testing.T) {
	tests := []struct {
		name    string
		command *nasMessage.ConfigurationUpdateCommand
		want    bool
	}{
		{name: "nil command"},
		{name: "indication absent", command: nasMessage.NewConfigurationUpdateCommand(0)},
		{
			name: "ack not requested",
			command: &nasMessage.ConfigurationUpdateCommand{
				ConfigurationUpdateIndication: configurationUpdateIndication(0),
			},
		},
		{
			name: "ack requested",
			command: &nasMessage.ConfigurationUpdateCommand{
				ConfigurationUpdateIndication: configurationUpdateIndication(1),
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := configurationUpdateNeedsAck(tt.command); got != tt.want {
				t.Fatalf("configurationUpdateNeedsAck() = %t, want %t", got, tt.want)
			}
		})
	}
}

func configurationUpdateIndication(ack uint8) *nasType.ConfigurationUpdateIndication {
	indication := nasType.NewConfigurationUpdateIndication(
		nasMessage.ConfigurationUpdateCommandConfigurationUpdateIndicationType,
	)
	indication.SetACK(ack)
	return indication
}
