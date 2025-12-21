# 🔐 Cloud Storage & Enhanced Security - Connectivity Extensions

## Overview
This document extends `CONNECTIVITY_ARCHITECTURE.md` with:
1. **Cloud Storage Connectors** (AWS S3, Azure Blob, Google Cloud Storage)
2. **Enhanced TCP/MLLP** with TLS and comprehensive authentication
3. **Mirth-style Cron UI** for user-friendly scheduling
4. **Universal Authentication Framework**

---

## ☁️ Cloud Storage Connectors

### **AWS S3 Connector** (Inbound - Cron-based)

```json
{
  "type_name": "aws_s3_inbound",
  "category": "inbound",
  "display_name": "AWS S3 Bucket Reader",
  "description": "Poll AWS S3 bucket for new objects (scheduled)",
  "icon": "☁️",
  "mode": "pull",
  "supports_cron": true,
  "requires_auth": true,

  "config_schema": {
    "type": "object",
    "required": ["bucket_name", "region"],
    "properties": {
      "bucket_name": {
        "type": "string",
        "title": "S3 Bucket Name",
        "description": "Name of the S3 bucket",
        "examples": ["my-hl7-bucket", "healthcare-inbound"]
      },
      "region": {
        "type": "string",
        "title": "AWS Region",
        "description": "AWS region where bucket is located",
        "enum": [
          "us-east-1", "us-east-2", "us-west-1", "us-west-2",
          "eu-west-1", "eu-central-1", "ap-southeast-1", "ap-northeast-1"
        ],
        "default": "us-east-1"
      },
      "prefix": {
        "type": "string",
        "title": "Object Prefix/Folder",
        "description": "Folder path within bucket (optional)",
        "examples": ["inbound/hl7/", "messages/2025/"]
      },
      "file_pattern": {
        "type": "string",
        "default": "*.hl7",
        "title": "File Pattern",
        "description": "Glob pattern for objects to process",
        "examples": ["*.hl7", "*.json", "ADT_*.hl7"]
      },

      "authentication_method": {
        "type": "string",
        "enum": ["access_keys", "iam_role", "assumed_role"],
        "default": "access_keys",
        "title": "Authentication Method"
      },

      "access_key_id": {
        "type": "string",
        "title": "AWS Access Key ID",
        "description": "Required if using access_keys method"
      },
      "secret_access_key": {
        "type": "string",
        "format": "password",
        "title": "AWS Secret Access Key",
        "description": "Required if using access_keys method"
      },

      "iam_role_arn": {
        "type": "string",
        "title": "IAM Role ARN",
        "description": "ARN of IAM role to assume",
        "pattern": "^arn:aws:iam::",
        "examples": ["arn:aws:iam::123456789012:role/ezHealthKonnect"]
      },
      "external_id": {
        "type": "string",
        "title": "External ID",
        "description": "External ID for assumed role (optional)"
      },

      "after_processing": {
        "type": "string",
        "enum": ["delete", "move", "tag", "nothing"],
        "default": "move",
        "title": "After Processing",
        "description": "What to do with object after successful processing"
      },
      "archive_prefix": {
        "type": "string",
        "title": "Archive Prefix",
        "description": "Destination prefix for processed objects",
        "examples": ["archive/", "processed/2025/"]
      },
      "error_prefix": {
        "type": "string",
        "title": "Error Prefix",
        "description": "Destination prefix for failed objects",
        "default": "errors/"
      },

      "processing_tag_key": {
        "type": "string",
        "default": "processed",
        "title": "Processing Tag Key",
        "description": "Tag key to mark processed objects (if after_processing=tag)"
      },
      "processing_tag_value": {
        "type": "string",
        "default": "true",
        "title": "Processing Tag Value"
      },

      "max_objects_per_poll": {
        "type": "integer",
        "default": 100,
        "minimum": 1,
        "maximum": 1000,
        "title": "Max Objects Per Poll",
        "description": "Maximum objects to process per execution"
      },

      "enable_server_side_encryption": {
        "type": "boolean",
        "default": false,
        "title": "Enable SSE (Server-Side Encryption)"
      },
      "kms_key_id": {
        "type": "string",
        "title": "KMS Key ID",
        "description": "KMS key for SSE-KMS encryption"
      }
    }
  },

  "parameter_groups": {
    "basic": ["bucket_name", "region", "prefix", "file_pattern"],
    "authentication": [
      "authentication_method",
      "access_key_id", "secret_access_key",
      "iam_role_arn", "external_id"
    ],
    "processing": [
      "after_processing", "archive_prefix", "error_prefix",
      "processing_tag_key", "processing_tag_value",
      "max_objects_per_poll"
    ],
    "security": ["enable_server_side_encryption", "kms_key_id"]
  },

  "validation_rules": {
    "conditional_required": [
      {
        "if": {"authentication_method": "access_keys"},
        "then_required": ["access_key_id", "secret_access_key"]
      },
      {
        "if": {"authentication_method": "assumed_role"},
        "then_required": ["iam_role_arn"]
      },
      {
        "if": {"after_processing": "move"},
        "then_required": ["archive_prefix"]
      },
      {
        "if": {"enable_server_side_encryption": true},
        "then_required": ["kms_key_id"]
      }
    ]
  }
}
```

### **AWS S3 Connector** (Outbound)

```json
{
  "type_name": "aws_s3_outbound",
  "category": "outbound",
  "display_name": "AWS S3 Bucket Writer",
  "description": "Upload processed messages to AWS S3 bucket",
  "icon": "☁️",
  "mode": "push",
  "supports_cron": false,
  "requires_auth": true,

  "config_schema": {
    "type": "object",
    "required": ["bucket_name", "region"],
    "properties": {
      "bucket_name": {
        "type": "string",
        "title": "S3 Bucket Name"
      },
      "region": {
        "type": "string",
        "title": "AWS Region",
        "enum": [
          "us-east-1", "us-east-2", "us-west-1", "us-west-2",
          "eu-west-1", "eu-central-1", "ap-southeast-1", "ap-northeast-1"
        ]
      },
      "prefix": {
        "type": "string",
        "title": "Object Prefix/Folder",
        "description": "Destination folder path",
        "examples": ["outbound/fhir/", "processed/"]
      },
      "filename_pattern": {
        "type": "string",
        "default": "{message_id}_{timestamp}.json",
        "title": "Filename Pattern",
        "description": "Pattern for object names. Variables: {message_id}, {timestamp}, {date}, {interface_id}",
        "examples": [
          "{message_id}_{timestamp}.json",
          "{date}/{interface_id}/{message_id}.fhir",
          "messages/{timestamp}_output.hl7"
        ]
      },

      "authentication_method": {
        "type": "string",
        "enum": ["access_keys", "iam_role", "assumed_role"],
        "default": "access_keys",
        "title": "Authentication Method"
      },
      "access_key_id": {
        "type": "string",
        "title": "AWS Access Key ID"
      },
      "secret_access_key": {
        "type": "string",
        "format": "password",
        "title": "AWS Secret Access Key"
      },
      "iam_role_arn": {
        "type": "string",
        "title": "IAM Role ARN"
      },

      "content_type": {
        "type": "string",
        "default": "application/json",
        "title": "Content-Type",
        "examples": ["application/json", "application/fhir+json", "text/plain", "application/hl7-v2"]
      },
      "content_encoding": {
        "type": "string",
        "enum": ["none", "gzip", "deflate"],
        "default": "none",
        "title": "Content Encoding (Compression)"
      },

      "storage_class": {
        "type": "string",
        "enum": ["STANDARD", "INTELLIGENT_TIERING", "STANDARD_IA", "GLACIER", "GLACIER_DEEP_ARCHIVE"],
        "default": "STANDARD",
        "title": "S3 Storage Class"
      },

      "enable_server_side_encryption": {
        "type": "boolean",
        "default": true,
        "title": "Enable SSE"
      },
      "encryption_type": {
        "type": "string",
        "enum": ["AES256", "aws:kms"],
        "default": "AES256",
        "title": "Encryption Type"
      },
      "kms_key_id": {
        "type": "string",
        "title": "KMS Key ID (if using SSE-KMS)"
      },

      "object_tagging": {
        "type": "array",
        "title": "Object Tags",
        "items": {
          "type": "object",
          "properties": {
            "key": {"type": "string"},
            "value": {"type": "string"}
          }
        },
        "default": [
          {"key": "source", "value": "ezHealthKonnect"},
          {"key": "interface_id", "value": "{interface_id}"}
        ]
      },

      "acl": {
        "type": "string",
        "enum": ["private", "public-read", "authenticated-read"],
        "default": "private",
        "title": "Access Control List (ACL)"
      }
    }
  },

  "parameter_groups": {
    "basic": ["bucket_name", "region", "prefix", "filename_pattern"],
    "authentication": ["authentication_method", "access_key_id", "secret_access_key", "iam_role_arn"],
    "format": ["content_type", "content_encoding", "storage_class"],
    "security": ["enable_server_side_encryption", "encryption_type", "kms_key_id", "acl"],
    "advanced": ["object_tagging"]
  }
}
```

### **Azure Blob Storage Connector** (Inbound - Cron-based)

```json
{
  "type_name": "azure_blob_inbound",
  "category": "inbound",
  "display_name": "Azure Blob Storage Reader",
  "description": "Poll Azure Blob Storage container for new blobs",
  "icon": "☁️",
  "mode": "pull",
  "supports_cron": true,
  "requires_auth": true,

  "config_schema": {
    "type": "object",
    "required": ["storage_account_name", "container_name"],
    "properties": {
      "storage_account_name": {
        "type": "string",
        "title": "Storage Account Name",
        "description": "Azure Storage account name",
        "examples": ["myaccountstorage", "healthcaredata"]
      },
      "container_name": {
        "type": "string",
        "title": "Container Name",
        "description": "Name of the blob container",
        "examples": ["hl7-inbound", "healthcare-messages"]
      },
      "prefix": {
        "type": "string",
        "title": "Blob Prefix/Folder",
        "description": "Virtual folder path (optional)",
        "examples": ["inbound/", "messages/2025/"]
      },
      "file_pattern": {
        "type": "string",
        "default": "*.hl7",
        "title": "File Pattern",
        "examples": ["*.hl7", "*.json"]
      },

      "authentication_method": {
        "type": "string",
        "enum": ["connection_string", "shared_key", "sas_token", "managed_identity"],
        "default": "connection_string",
        "title": "Authentication Method"
      },

      "connection_string": {
        "type": "string",
        "format": "password",
        "title": "Connection String",
        "description": "Full Azure Storage connection string"
      },

      "account_key": {
        "type": "string",
        "format": "password",
        "title": "Account Key",
        "description": "Storage account access key (if using shared_key)"
      },

      "sas_token": {
        "type": "string",
        "format": "password",
        "title": "SAS Token",
        "description": "Shared Access Signature token"
      },

      "after_processing": {
        "type": "string",
        "enum": ["delete", "move", "tag", "nothing"],
        "default": "move",
        "title": "After Processing"
      },
      "archive_container": {
        "type": "string",
        "title": "Archive Container",
        "description": "Destination container for processed blobs"
      },
      "archive_prefix": {
        "type": "string",
        "title": "Archive Prefix",
        "description": "Destination prefix for processed blobs"
      },

      "max_blobs_per_poll": {
        "type": "integer",
        "default": 100,
        "minimum": 1,
        "maximum": 1000,
        "title": "Max Blobs Per Poll"
      }
    }
  },

  "parameter_groups": {
    "basic": ["storage_account_name", "container_name", "prefix", "file_pattern"],
    "authentication": [
      "authentication_method",
      "connection_string", "account_key", "sas_token"
    ],
    "processing": [
      "after_processing", "archive_container", "archive_prefix",
      "max_blobs_per_poll"
    ]
  },

  "validation_rules": {
    "conditional_required": [
      {
        "if": {"authentication_method": "connection_string"},
        "then_required": ["connection_string"]
      },
      {
        "if": {"authentication_method": "shared_key"},
        "then_required": ["account_key"]
      },
      {
        "if": {"authentication_method": "sas_token"},
        "then_required": ["sas_token"]
      },
      {
        "if": {"after_processing": "move"},
        "then_required": ["archive_container"]
      }
    ]
  }
}
```

### **Azure Blob Storage Connector** (Outbound)

```json
{
  "type_name": "azure_blob_outbound",
  "category": "outbound",
  "display_name": "Azure Blob Storage Writer",
  "description": "Upload processed messages to Azure Blob Storage",
  "icon": "☁️",
  "mode": "push",
  "supports_cron": false,
  "requires_auth": true,

  "config_schema": {
    "type": "object",
    "required": ["storage_account_name", "container_name"],
    "properties": {
      "storage_account_name": {
        "type": "string",
        "title": "Storage Account Name"
      },
      "container_name": {
        "type": "string",
        "title": "Container Name"
      },
      "prefix": {
        "type": "string",
        "title": "Blob Prefix/Folder"
      },
      "filename_pattern": {
        "type": "string",
        "default": "{message_id}_{timestamp}.json",
        "title": "Filename Pattern"
      },

      "authentication_method": {
        "type": "string",
        "enum": ["connection_string", "shared_key", "sas_token", "managed_identity"],
        "default": "connection_string",
        "title": "Authentication Method"
      },
      "connection_string": {
        "type": "string",
        "format": "password",
        "title": "Connection String"
      },
      "account_key": {
        "type": "string",
        "format": "password",
        "title": "Account Key"
      },
      "sas_token": {
        "type": "string",
        "format": "password",
        "title": "SAS Token"
      },

      "content_type": {
        "type": "string",
        "default": "application/json",
        "title": "Content-Type"
      },
      "blob_type": {
        "type": "string",
        "enum": ["BlockBlob", "AppendBlob"],
        "default": "BlockBlob",
        "title": "Blob Type"
      },
      "access_tier": {
        "type": "string",
        "enum": ["Hot", "Cool", "Archive"],
        "default": "Hot",
        "title": "Access Tier"
      },

      "blob_metadata": {
        "type": "array",
        "title": "Blob Metadata (Key-Value Pairs)",
        "items": {
          "type": "object",
          "properties": {
            "key": {"type": "string"},
            "value": {"type": "string"}
          }
        },
        "default": [
          {"key": "source", "value": "ezHealthKonnect"},
          {"key": "interface_id", "value": "{interface_id}"}
        ]
      }
    }
  },

  "parameter_groups": {
    "basic": ["storage_account_name", "container_name", "prefix", "filename_pattern"],
    "authentication": ["authentication_method", "connection_string", "account_key", "sas_token"],
    "format": ["content_type", "blob_type", "access_tier"],
    "advanced": ["blob_metadata"]
  }
}
```

### **Google Cloud Storage Connector** (Inbound - Cron-based)

```json
{
  "type_name": "gcs_inbound",
  "category": "inbound",
  "display_name": "Google Cloud Storage Reader",
  "description": "Poll GCS bucket for new objects",
  "icon": "☁️",
  "mode": "pull",
  "supports_cron": true,
  "requires_auth": true,

  "config_schema": {
    "type": "object",
    "required": ["bucket_name", "project_id"],
    "properties": {
      "project_id": {
        "type": "string",
        "title": "GCP Project ID",
        "description": "Google Cloud project ID",
        "examples": ["my-healthcare-project", "prod-integrations"]
      },
      "bucket_name": {
        "type": "string",
        "title": "Bucket Name",
        "description": "Name of the GCS bucket",
        "examples": ["hl7-inbound-bucket", "healthcare-messages"]
      },
      "prefix": {
        "type": "string",
        "title": "Object Prefix/Folder",
        "examples": ["inbound/", "messages/"]
      },
      "file_pattern": {
        "type": "string",
        "default": "*.hl7",
        "title": "File Pattern"
      },

      "authentication_method": {
        "type": "string",
        "enum": ["service_account_json", "service_account_file", "application_default"],
        "default": "service_account_json",
        "title": "Authentication Method"
      },

      "service_account_json": {
        "type": "string",
        "format": "password",
        "title": "Service Account JSON",
        "description": "JSON content of service account key file"
      },

      "service_account_file_path": {
        "type": "string",
        "title": "Service Account Key File Path",
        "description": "Path to service account JSON key file"
      },

      "after_processing": {
        "type": "string",
        "enum": ["delete", "move", "nothing"],
        "default": "move",
        "title": "After Processing"
      },
      "archive_bucket": {
        "type": "string",
        "title": "Archive Bucket",
        "description": "Destination bucket for processed objects (optional, uses same bucket if not specified)"
      },
      "archive_prefix": {
        "type": "string",
        "title": "Archive Prefix",
        "description": "Destination prefix for processed objects"
      },

      "max_objects_per_poll": {
        "type": "integer",
        "default": 100,
        "minimum": 1,
        "maximum": 1000,
        "title": "Max Objects Per Poll"
      }
    }
  },

  "parameter_groups": {
    "basic": ["project_id", "bucket_name", "prefix", "file_pattern"],
    "authentication": [
      "authentication_method",
      "service_account_json",
      "service_account_file_path"
    ],
    "processing": [
      "after_processing", "archive_bucket", "archive_prefix",
      "max_objects_per_poll"
    ]
  },

  "validation_rules": {
    "conditional_required": [
      {
        "if": {"authentication_method": "service_account_json"},
        "then_required": ["service_account_json"]
      },
      {
        "if": {"authentication_method": "service_account_file"},
        "then_required": ["service_account_file_path"]
      },
      {
        "if": {"after_processing": "move"},
        "then_required": ["archive_prefix"]
      }
    ]
  }
}
```

### **Google Cloud Storage Connector** (Outbound)

```json
{
  "type_name": "gcs_outbound",
  "category": "outbound",
  "display_name": "Google Cloud Storage Writer",
  "description": "Upload processed messages to GCS bucket",
  "icon": "☁️",
  "mode": "push",
  "supports_cron": false,
  "requires_auth": true,

  "config_schema": {
    "type": "object",
    "required": ["bucket_name", "project_id"],
    "properties": {
      "project_id": {
        "type": "string",
        "title": "GCP Project ID"
      },
      "bucket_name": {
        "type": "string",
        "title": "Bucket Name"
      },
      "prefix": {
        "type": "string",
        "title": "Object Prefix/Folder"
      },
      "filename_pattern": {
        "type": "string",
        "default": "{message_id}_{timestamp}.json",
        "title": "Filename Pattern"
      },

      "authentication_method": {
        "type": "string",
        "enum": ["service_account_json", "service_account_file", "application_default"],
        "default": "service_account_json",
        "title": "Authentication Method"
      },
      "service_account_json": {
        "type": "string",
        "format": "password",
        "title": "Service Account JSON"
      },
      "service_account_file_path": {
        "type": "string",
        "title": "Service Account Key File Path"
      },

      "content_type": {
        "type": "string",
        "default": "application/json",
        "title": "Content-Type"
      },
      "storage_class": {
        "type": "string",
        "enum": ["STANDARD", "NEARLINE", "COLDLINE", "ARCHIVE"],
        "default": "STANDARD",
        "title": "Storage Class"
      },

      "object_metadata": {
        "type": "array",
        "title": "Object Metadata",
        "items": {
          "type": "object",
          "properties": {
            "key": {"type": "string"},
            "value": {"type": "string"}
          }
        },
        "default": [
          {"key": "source", "value": "ezHealthKonnect"},
          {"key": "interface_id", "value": "{interface_id}"}
        ]
      }
    }
  },

  "parameter_groups": {
    "basic": ["project_id", "bucket_name", "prefix", "filename_pattern"],
    "authentication": ["authentication_method", "service_account_json", "service_account_file_path"],
    "format": ["content_type", "storage_class"],
    "advanced": ["object_metadata"]
  }
}
```

---

## 🔐 Enhanced TCP/MLLP with TLS & Authentication

### **TCP/MLLP Connector** (Enhanced with TLS + Auth)

```json
{
  "type_name": "tcp_mllp",
  "category": "inbound",
  "display_name": "TCP/MLLP (HL7 v2.x)",
  "description": "Receive HL7 v2.x messages over MLLP protocol with TLS and authentication",
  "icon": "🔌",
  "mode": "push",
  "supports_cron": false,
  "requires_auth": true,

  "config_schema": {
    "type": "object",
    "required": ["port"],
    "properties": {
      "port": {
        "type": "integer",
        "minimum": 1024,
        "maximum": 65535,
        "default": 2575,
        "title": "Listen Port",
        "description": "Port to listen for MLLP connections"
      },
      "host": {
        "type": "string",
        "default": "0.0.0.0",
        "title": "Bind Address",
        "description": "IP address to bind (0.0.0.0 for all interfaces)"
      },
      "max_connections": {
        "type": "integer",
        "default": 10,
        "minimum": 1,
        "maximum": 100,
        "title": "Max Concurrent Connections",
        "description": "Maximum number of simultaneous connections"
      },
      "timeout_seconds": {
        "type": "integer",
        "default": 300,
        "minimum": 10,
        "maximum": 3600,
        "title": "Connection Timeout (seconds)",
        "description": "Idle connection timeout"
      },
      "buffer_size_kb": {
        "type": "integer",
        "default": 64,
        "minimum": 4,
        "maximum": 1024,
        "title": "Buffer Size (KB)",
        "description": "Read buffer size for incoming messages"
      },

      "enable_tls": {
        "type": "boolean",
        "default": false,
        "title": "Enable TLS/SSL Encryption"
      },
      "tls_version": {
        "type": "string",
        "enum": ["TLS 1.2", "TLS 1.3", "TLS 1.2+"],
        "default": "TLS 1.2+",
        "title": "Minimum TLS Version",
        "description": "Minimum TLS protocol version"
      },
      "tls_cert_path": {
        "type": "string",
        "title": "TLS Certificate Path",
        "description": "Path to PEM-encoded certificate file",
        "examples": ["/etc/ssl/certs/server.crt", "C:\\certs\\server.pem"]
      },
      "tls_key_path": {
        "type": "string",
        "title": "TLS Private Key Path",
        "description": "Path to PEM-encoded private key file",
        "examples": ["/etc/ssl/private/server.key", "C:\\certs\\server.key"]
      },
      "tls_ca_cert_path": {
        "type": "string",
        "title": "TLS CA Certificate Path (Optional)",
        "description": "Path to CA certificate for client certificate verification"
      },
      "tls_verify_client": {
        "type": "boolean",
        "default": false,
        "title": "Verify Client Certificate (Mutual TLS)",
        "description": "Require and verify client certificates"
      },

      "enable_authentication": {
        "type": "boolean",
        "default": false,
        "title": "Enable Authentication",
        "description": "Require authentication for connections"
      },
      "authentication_method": {
        "type": "string",
        "enum": ["basic", "token", "certificate", "ip_whitelist"],
        "default": "basic",
        "title": "Authentication Method"
      },

      "username": {
        "type": "string",
        "title": "Username (Basic Auth)",
        "description": "Required username for basic authentication"
      },
      "password": {
        "type": "string",
        "format": "password",
        "title": "Password (Basic Auth)",
        "description": "Required password for basic authentication"
      },

      "auth_token": {
        "type": "string",
        "format": "password",
        "title": "Authentication Token",
        "description": "Shared secret token for token-based auth"
      },

      "ip_whitelist": {
        "type": "array",
        "title": "IP Whitelist",
        "description": "Allowed IP addresses or CIDR ranges",
        "items": {
          "type": "string",
          "pattern": "^([0-9]{1,3}\\.){3}[0-9]{1,3}(/[0-9]{1,2})?$"
        },
        "examples": [
          ["192.168.1.100", "192.168.1.0/24", "10.0.0.0/8"]
        ]
      },

      "enable_keepalive": {
        "type": "boolean",
        "default": true,
        "title": "Enable TCP Keep-Alive"
      },
      "keepalive_interval_seconds": {
        "type": "integer",
        "default": 60,
        "title": "Keep-Alive Interval (seconds)"
      },

      "log_connections": {
        "type": "boolean",
        "default": true,
        "title": "Log Connection Events",
        "description": "Log connection/disconnection events"
      },
      "log_full_messages": {
        "type": "boolean",
        "default": false,
        "title": "Log Full Messages (Debug Only)",
        "description": "WARNING: May expose PHI - use only for debugging"
      }
    }
  },

  "parameter_groups": {
    "basic": ["port", "host", "max_connections", "timeout_seconds"],
    "tls_encryption": [
      "enable_tls", "tls_version",
      "tls_cert_path", "tls_key_path",
      "tls_ca_cert_path", "tls_verify_client"
    ],
    "authentication": [
      "enable_authentication", "authentication_method",
      "username", "password",
      "auth_token", "ip_whitelist"
    ],
    "advanced": [
      "buffer_size_kb",
      "enable_keepalive", "keepalive_interval_seconds",
      "log_connections", "log_full_messages"
    ]
  },

  "validation_rules": {
    "conditional_required": [
      {
        "if": {"enable_tls": true},
        "then_required": ["tls_cert_path", "tls_key_path"]
      },
      {
        "if": {"tls_verify_client": true},
        "then_required": ["tls_ca_cert_path"]
      },
      {
        "if": {"enable_authentication": true, "authentication_method": "basic"},
        "then_required": ["username", "password"]
      },
      {
        "if": {"enable_authentication": true, "authentication_method": "token"},
        "then_required": ["auth_token"]
      },
      {
        "if": {"enable_authentication": true, "authentication_method": "ip_whitelist"},
        "then_required": ["ip_whitelist"]
      }
    ]
  }
}
```

---

## 📅 Mirth-Style Cron UI Configuration

### **User-Friendly Polling Schedule Interface**

```javascript
// UI Component Structure (similar to Mirth Connect)

{
  "component": "PollingScheduler",
  "style": "mirth",

  "modes": [
    {
      "mode": "simple",
      "display_name": "Simple",
      "description": "Easy-to-use interface for common schedules",

      "fields": [
        {
          "type": "radio",
          "name": "frequency",
          "label": "How often should this run?",
          "options": [
            {
              "value": "minutes",
              "label": "Every X minutes",
              "sub_field": {
                "type": "number",
                "name": "interval_minutes",
                "min": 1,
                "max": 1440,
                "default": 5,
                "suffix": "minute(s)",
                "generates": "*/X * * * *"
              }
            },
            {
              "value": "hourly",
              "label": "Every X hours at minute",
              "sub_fields": [
                {
                  "type": "number",
                  "name": "interval_hours",
                  "min": 1,
                  "max": 23,
                  "default": 1,
                  "label": "Every",
                  "suffix": "hour(s)"
                },
                {
                  "type": "number",
                  "name": "minute",
                  "min": 0,
                  "max": 59,
                  "default": 0,
                  "label": "at minute",
                  "generates": "M */H * * *"
                }
              ]
            },
            {
              "value": "daily",
              "label": "Daily at specific time",
              "sub_field": {
                "type": "time",
                "name": "time",
                "default": "09:00",
                "generates": "M H * * *"
              }
            },
            {
              "value": "weekly",
              "label": "Weekly on specific days",
              "sub_fields": [
                {
                  "type": "checkbox_group",
                  "name": "days",
                  "label": "Select days:",
                  "options": [
                    {"value": "0", "label": "Sunday"},
                    {"value": "1", "label": "Monday"},
                    {"value": "2", "label": "Tuesday"},
                    {"value": "3", "label": "Wednesday"},
                    {"value": "4", "label": "Thursday"},
                    {"value": "5", "label": "Friday"},
                    {"value": "6", "label": "Saturday"}
                  ],
                  "default": ["1", "2", "3", "4", "5"]
                },
                {
                  "type": "time",
                  "name": "time",
                  "default": "09:00",
                  "generates": "M H * * D,D,D"
                }
              ]
            },
            {
              "value": "monthly",
              "label": "Monthly on specific day",
              "sub_fields": [
                {
                  "type": "radio",
                  "name": "day_selection",
                  "options": [
                    {
                      "value": "day_of_month",
                      "label": "On day",
                      "sub_field": {
                        "type": "number",
                        "name": "day",
                        "min": 1,
                        "max": 31,
                        "default": 1
                      }
                    },
                    {
                      "value": "last_day",
                      "label": "Last day of month"
                    }
                  ]
                },
                {
                  "type": "time",
                  "name": "time",
                  "default": "09:00",
                  "generates": "M H D * *"
                }
              ]
            }
          ]
        },

        {
          "type": "select",
          "name": "timezone",
          "label": "Timezone",
          "options": [
            {"value": "UTC", "label": "UTC (Coordinated Universal Time)"},
            {"value": "America/New_York", "label": "Eastern Time (US & Canada)"},
            {"value": "America/Chicago", "label": "Central Time (US & Canada)"},
            {"value": "America/Denver", "label": "Mountain Time (US & Canada)"},
            {"value": "America/Los_Angeles", "label": "Pacific Time (US & Canada)"},
            {"value": "Europe/London", "label": "London (GMT/BST)"},
            {"value": "Europe/Paris", "label": "Central European Time"},
            {"value": "Asia/Tokyo", "label": "Tokyo (JST)"},
            {"value": "Australia/Sydney", "label": "Sydney (AEST)"}
          ],
          "default": "America/New_York"
        }
      ],

      "preview": {
        "show": true,
        "label": "Next 5 scheduled runs:",
        "format": "YYYY-MM-DD HH:mm:ss Z"
      }
    },

    {
      "mode": "advanced",
      "display_name": "Advanced (Cron Expression)",
      "description": "For users familiar with cron syntax",

      "fields": [
        {
          "type": "text",
          "name": "cron_expression",
          "label": "Cron Expression",
          "placeholder": "*/5 * * * *",
          "help_text": "Format: minute hour day-of-month month day-of-week",
          "validation": "cron_syntax",
          "examples": [
            {
              "expression": "*/5 * * * *",
              "description": "Every 5 minutes"
            },
            {
              "expression": "0 * * * *",
              "description": "Every hour at minute 0"
            },
            {
              "expression": "0 9 * * 1-5",
              "description": "Weekdays at 9:00 AM"
            },
            {
              "expression": "0 0 1 * *",
              "description": "First day of every month at midnight"
            },
            {
              "expression": "*/15 9-17 * * 1-5",
              "description": "Every 15 minutes during business hours (9 AM - 5 PM) on weekdays"
            }
          ]
        },

        {
          "type": "select",
          "name": "timezone",
          "label": "Timezone",
          "options": "...(same as simple mode)"
        }
      ],

      "preview": {
        "show": true,
        "label": "Next 5 scheduled runs:",
        "format": "YYYY-MM-DD HH:mm:ss Z"
      },

      "tools": {
        "cron_builder": true,
        "expression_tester": true
      }
    }
  ],

  "live_preview": {
    "enabled": true,
    "update_delay_ms": 500,
    "show_human_readable": true,
    "show_next_runs": 5
  },

  "validation": {
    "on_change": true,
    "show_errors_inline": true,
    "test_parse": true
  }
}
```

### **Visual Schedule Builder (Drag & Drop)**

```javascript
// Mirth-style visual scheduler
{
  "component": "VisualScheduleBuilder",

  "ui_elements": [
    {
      "element": "frequency_selector",
      "type": "large_buttons",
      "options": [
        {
          "icon": "⏱️",
          "label": "Minutes",
          "sublabel": "Run every X minutes"
        },
        {
          "icon": "🕐",
          "label": "Hours",
          "sublabel": "Run every X hours"
        },
        {
          "icon": "📅",
          "label": "Daily",
          "sublabel": "Run once per day"
        },
        {
          "icon": "📆",
          "label": "Weekly",
          "sublabel": "Run on specific days"
        },
        {
          "icon": "🗓️",
          "label": "Monthly",
          "sublabel": "Run once per month"
        },
        {
          "icon": "⚙️",
          "label": "Custom",
          "sublabel": "Advanced cron expression"
        }
      ]
    },

    {
      "element": "time_picker",
      "type": "clock_visual",
      "show_12_hour": true,
      "show_24_hour": true,
      "allow_toggle": true
    },

    {
      "element": "day_selector",
      "type": "week_calendar",
      "visual": "clickable_days",
      "highlight_selected": true,
      "show_weekday_names": true
    },

    {
      "element": "preview_panel",
      "position": "right_sidebar",
      "realtime": true,
      "content": [
        {
          "section": "human_readable",
          "example": "Runs every 5 minutes"
        },
        {
          "section": "cron_expression",
          "example": "*/5 * * * *",
          "editable": true
        },
        {
          "section": "next_runs",
          "count": 5,
          "format": "relative_and_absolute",
          "examples": [
            "In 2 minutes (2025-10-25 10:30:00 EST)",
            "In 7 minutes (2025-10-25 10:35:00 EST)",
            "In 12 minutes (2025-10-25 10:40:00 EST)"
          ]
        }
      ]
    }
  ]
}
```

---

## 🔐 Universal Authentication Framework

### **Authentication Configuration Schema**

```typescript
// Universal auth config that works across all connectors

interface AuthenticationConfig {
  // Basic/Simple Auth
  basic_auth?: {
    username: string;
    password: string;  // encrypted at rest
  };

  // API Key/Token Auth
  api_key_auth?: {
    header_name: string;  // e.g., "X-API-Key", "Authorization"
    key_value: string;    // encrypted at rest
    prefix?: string;      // e.g., "Bearer ", "Token "
  };

  // OAuth 2.0
  oauth2?: {
    grant_type: 'client_credentials' | 'authorization_code' | 'password';
    client_id: string;
    client_secret: string;  // encrypted
    token_url: string;
    scope?: string;
    refresh_token?: string; // encrypted
    expires_at?: Date;
  };

  // Certificate-based (Mutual TLS)
  certificate?: {
    cert_path: string;
    key_path: string;
    ca_cert_path?: string;
    verify_server: boolean;
  };

  // Cloud Provider Credentials
  aws?: {
    method: 'access_keys' | 'iam_role' | 'assumed_role';
    access_key_id?: string;
    secret_access_key?: string;  // encrypted
    role_arn?: string;
    external_id?: string;
    region: string;
  };

  azure?: {
    method: 'connection_string' | 'shared_key' | 'sas_token' | 'managed_identity';
    connection_string?: string;  // encrypted
    account_key?: string;        // encrypted
    sas_token?: string;          // encrypted
  };

  gcp?: {
    method: 'service_account_json' | 'service_account_file' | 'application_default';
    service_account_json?: string;    // encrypted
    service_account_file_path?: string;
    project_id: string;
  };

  // IP Whitelist (no credentials needed)
  ip_whitelist?: {
    allowed_ips: string[];  // CIDR format supported
    deny_by_default: boolean;
  };

  // Custom headers
  custom_headers?: Array<{
    name: string;
    value: string;  // encrypted if contains sensitive data
    is_sensitive: boolean;
  }>;
}
```

---

## 📊 Summary

### **Comprehensive Coverage**

**Cloud Storage: 6 Connectors** ✅
- AWS S3 (Inbound + Outbound)
- Azure Blob Storage (Inbound + Outbound)
- Google Cloud Storage (Inbound + Outbound)

**Enhanced Security** ✅
- TLS 1.2/1.3 support for TCP/MLLP
- Mutual TLS (client certificate verification)
- Multiple authentication methods per connector
- IP whitelisting
- Encrypted credential storage

**Mirth-Style Cron UI** ✅
- Simple mode (visual builder)
- Advanced mode (cron expression)
- Live preview of next runs
- Timezone support
- Human-readable descriptions

**Universal Authentication** ✅
- Basic Auth
- API Keys/Tokens
- OAuth 2.0
- Certificate-based (mTLS)
- Cloud-specific (AWS, Azure, GCP)
- IP Whitelist
- Custom headers

---

*Extension Document: 2025-10-25*
*Complements: CONNECTIVITY_ARCHITECTURE.md*
