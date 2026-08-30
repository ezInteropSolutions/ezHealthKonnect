// services/connectors/aws_s3_outbound_test.go
// Coverage for buildKey and its supporting helpers -- pure logic that doesn't
// need a live S3 bucket, so it's tested directly without Initialize().
package connectors

import (
	"strings"
	"testing"

	"ezhealthkonnect/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func newTestS3Outbound(cfg AWSS3OutboundConfig) *AWSS3OutboundConnector {
	return &AWSS3OutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(ConnectorMetadata{TypeName: "aws_s3_outbound"}, true),
		config:                cfg,
	}
}

func TestBuildKey_DefaultPattern(t *testing.T) {
	c := newTestS3Outbound(AWSS3OutboundConfig{KeyPattern: "{message_id}.json"})
	msg := &models.OutboundMessage{MessageID: "abc123"}
	key := c.buildKey(msg)
	if key != "abc123.json" {
		t.Errorf("expected 'abc123.json', got %q", key)
	}
}

func TestBuildKey_WithPrefixAndDate(t *testing.T) {
	c := newTestS3Outbound(AWSS3OutboundConfig{KeyPattern: "outbound/{date}/{message_id}.hl7"})
	msg := &models.OutboundMessage{MessageID: "msg-1"}
	key := c.buildKey(msg)
	if !strings.HasPrefix(key, "outbound/") || !strings.HasSuffix(key, "/msg-1.hl7") {
		t.Errorf("expected prefix+date+message_id pattern to render, got %q", key)
	}
}

func TestBuildKey_SanitizesUnsafeCharacters(t *testing.T) {
	c := newTestS3Outbound(AWSS3OutboundConfig{KeyPattern: "{message_id}.json"})
	msg := &models.OutboundMessage{MessageID: "has/slash:colon"}
	key := c.buildKey(msg)
	if key != "has_slash_colon.json" {
		t.Errorf("expected unsafe characters in message_id replaced with underscores, got %q", key)
	}
}

func TestBuildKey_NoExtensionInPattern_InfersFromContentType(t *testing.T) {
	c := newTestS3Outbound(AWSS3OutboundConfig{KeyPattern: "{message_id}"})
	msg := &models.OutboundMessage{MessageID: "abc", ContentType: "application/hl7-v2"}
	key := c.buildKey(msg)
	if key != "abc.hl7" {
		t.Errorf("expected content-type-inferred .hl7 extension, got %q", key)
	}
}

func TestBuildKey_ExplicitExtensionInPatternWins(t *testing.T) {
	c := newTestS3Outbound(AWSS3OutboundConfig{KeyPattern: "{message_id}.xml"})
	msg := &models.OutboundMessage{MessageID: "abc", ContentType: "application/json"}
	key := c.buildKey(msg)
	if key != "abc.xml" {
		t.Errorf("a pattern with its own extension must not be overridden by content-type inference, got %q", key)
	}
}

func TestExtensionForContentType(t *testing.T) {
	cases := map[string]string{
		"application/hl7-v2": ".hl7",
		"application/json":   ".json",
		"application/fhir+json": ".json",
		"application/xml":   ".xml",
		"text/csv":          ".csv",
		"text/plain":        ".txt",
		"":                  ".txt",
	}
	for ct, want := range cases {
		if got := extensionForContentType(ct); got != want {
			t.Errorf("extensionForContentType(%q) = %q, want %q", ct, got, want)
		}
	}
}

func TestApplyEncryption_AES256(t *testing.T) {
	c := newTestS3Outbound(AWSS3OutboundConfig{ServerSideEncryption: "AES256"})
	input := &s3.PutObjectInput{}
	c.applyEncryption(input)
	if input.ServerSideEncryption != types.ServerSideEncryptionAes256 {
		t.Errorf("expected AES256 server-side encryption set, got %v", input.ServerSideEncryption)
	}
}

func TestApplyEncryption_KMS(t *testing.T) {
	c := newTestS3Outbound(AWSS3OutboundConfig{ServerSideEncryption: "aws:kms", KMSKeyID: "arn:aws:kms:us-east-1:123:key/abc"})
	input := &s3.PutObjectInput{}
	c.applyEncryption(input)
	if input.ServerSideEncryption != types.ServerSideEncryptionAwsKms {
		t.Errorf("expected aws:kms server-side encryption set, got %v", input.ServerSideEncryption)
	}
	if aws.ToString(input.SSEKMSKeyId) != "arn:aws:kms:us-east-1:123:key/abc" {
		t.Errorf("expected KMS key id passed through, got %v", input.SSEKMSKeyId)
	}
}

func TestApplyEncryption_None(t *testing.T) {
	c := newTestS3Outbound(AWSS3OutboundConfig{ServerSideEncryption: "none"})
	input := &s3.PutObjectInput{}
	c.applyEncryption(input)
	if input.ServerSideEncryption != "" {
		t.Errorf("expected no server-side encryption set when \"none\" is configured, got %v", input.ServerSideEncryption)
	}
}

func TestAWSS3OutboundValidate_RequiresBucketAndRegion(t *testing.T) {
	c := newTestS3Outbound(AWSS3OutboundConfig{})
	c.BaseOutboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err == nil {
		t.Error("Validate must reject missing bucket and region")
	}

	c2 := newTestS3Outbound(AWSS3OutboundConfig{Bucket: "b", Region: "us-east-1"})
	c2.BaseOutboundConnector.BaseConnector.initialized = true
	if err := c2.Validate(); err != nil {
		t.Errorf("Validate should pass with bucket and region set, got: %v", err)
	}
}
