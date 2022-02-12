package cmd

import (
	"errors"
	"testing"
)

//func TestCommand(t *testing.T) {
//	b := bytes.NewBufferString("")
//	rootCmd.SetOut(b)
//	rootCmd.Execute()
//	out, err := ioutil.ReadAll(b)
//	if err != nil {
//		log.Fatal(err)
//	}

//}

func TestSanitize(t *testing.T) {

	var tests = []struct {
		given         string
		expected      string
		expectedError error
	}{
		{"2launch.us", "rootdomain-2launch-us", nil},
		{"launch.us", "launch-us", nil},
		{"valid.launch.us", "valid-launch-us", nil},
		{"invalid", "invalid", errors.New("anything")},
	}
	for _, tt := range tests {
		tt := tt
		t.Run("", func(t *testing.T) {
			actual, err := sanitize(tt.given)
			if tt.expectedError != nil {
				if err == nil {
					t.Fatal("expected error but got no error")
				}

			} else {
				if actual != tt.expected {
					t.Errorf("(%s): expected %s, actual %s", tt.given, tt.expected, actual)
				}
			}

		})
	}
}
