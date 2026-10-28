package ai

import "testing"

func TestExtractHL7VersionAndTypeStayInsideMSH(t *testing.T) {
	cases := []struct{ name, msg, wantType, wantVer string }{
		{"CR separated", "MSH|^~\\&|A|B|C|D|20260101||ADT^A01|1|P|2.5\rPID|1||123", "ADT^A01", "2.5"},
		{"CRLF separated", "MSH|^~\\&|A|B|C|D|20260101||ORU^R01|1|P|2.5.1\r\nPID|1", "ORU^R01", "2.5.1"},
		{"LF separated", "MSH|^~\\&|A|B|C|D|20260101||ADT^A08|1|P|2.3\nPID|1", "ADT^A08", "2.3"},
		{"version with components", "MSH|^~\\&|A|B|C|D|20260101||ADT^A01|1|P|2.5.1^US\rPID|1", "ADT^A01", "2.5.1"},
		{"MSH only", "MSH|^~\\&|A|B|C|D|20260101||ADT^A01|1|P|2.5", "ADT^A01", "2.5"},
		{"short MSH does not borrow from the next segment", "MSH|^~\\&|A\rPID|1|2|3|4|5|6|7|8|9|10|11|12", "", ""},
	}
	for _, c := range cases {
		if got := extractHL7MsgType(c.msg); got != c.wantType {
			t.Errorf("%s: type = %q, want %q", c.name, got, c.wantType)
		}
		if got := extractHL7Version(c.msg); got != c.wantVer {
			t.Errorf("%s: version = %q, want %q", c.name, got, c.wantVer)
		}
	}
}
