# Silo Multi-user Quickstart Guide

Silo supports multiple long term users in addition to default user created during server startup. New users can be added after server starts up, and server can be configured to deny or allow access to buckets and resources to each of these users. This document explains how to add/remove users and modify their access rights.

## Get started

In this document we will explain in detail on how to configure multiple users.

### 1. Prerequisites

- Install mc - [Silo Client Quickstart Guide](https://silo.pgsty.com/reference/minio-mc/#quickstart)
- Install Silo - [Silo Quickstart Guide](https://silo.pgsty.com/operations/deployments/baremetal-deploy-minio-on-redhat-linux/)
- Configure etcd - [Etcd V3 Quickstart Guide](https://github.com/pgsty/silo/blob/main/docs/sts/etcd.md)

### 2. Create a new user with canned policy

Use [`mc admin policy`](https://silo.pgsty.com/reference/minio-mc-admin/mc-admin-policy/) to create canned policies. Server provides a default set of canned policies namely `writeonly`, `readonly` and `readwrite` *(these policies apply to all resources on the server)*. These can be overridden by custom policies using `mc admin policy` command.

Create new canned policy file `getonly.json`. This policy enables users to download all objects under `my-bucketname`.

```json
cat > getonly.json << EOF
{
  "Version": "2012-10-17",
  "Statement": [
	{
	  "Action": [
		"s3:GetObject"
	  ],
	  "Effect": "Allow",
	  "Resource": [
		"arn:aws:s3:::my-bucketname/*"
	  ],
	  "Sid": ""
	}
  ]
}
EOF
```

Create new canned policy by name `getonly` using `getonly.json` policy file.

```
mc admin policy create mysilo getonly getonly.json
```

Create a new user `newuser` on Silo use `mc admin user`.

```
mc admin user add mysilo newuser newuser123
```

Once the user is successfully created you can now apply the `getonly` policy for this user.

```
mc admin policy attach mysilo getonly --user=newuser
```

### 3. Create a new group

```
mc admin group add mysilo newgroup newuser
```

Once the group is successfully created you can now apply the `getonly` policy for this group.

```
mc admin policy attach mysilo getonly --group=newgroup
```

### 4. Disable user

Disable user `newuser`.

```
mc admin user disable mysilo newuser
```

Disable group `newgroup`.

```
mc admin group disable mysilo newgroup
```

### 5. Remove user

Remove the user `newuser`.

```
mc admin user remove mysilo newuser
```

Remove the user `newuser` from a group.

```
mc admin group remove mysilo newgroup newuser
```

Remove the group `newgroup`.

```
mc admin group remove mysilo newgroup
```

### 6. Change user or group policy

Change the policy for user `newuser` to `putonly` canned policy.

```
mc admin policy attach mysilo putonly --user=newuser
```

Change the policy for group `newgroup` to `putonly` canned policy.

```
mc admin policy attach mysilo putonly --group=newgroup
```

### 7. List all users or groups

List all enabled and disabled users.

```
mc admin user list mysilo
```

List all enabled or disabled groups.

```
mc admin group list mysilo
```

### 8. Configure `mc`

```
mc alias set mysilo-newuser http://localhost:9000 newuser newuser123 --api s3v4
mc cat mysilo-newuser/my-bucketname/my-objectname
```

### Policy Variables

You can use policy variables in the *Resource* element and in string comparisons in the *Condition* element.

You can use a policy variable in the Resource element, but only in the resource portion of the ARN. This portion of the ARN appears after the 5th colon (:). You can't use a variable to replace parts of the ARN before the 5th colon, such as the service or account. The following policy might be attached to a group. It gives each of the users in the group full programmatic access to a user-specific object (their own "home directory") in Silo.

```
{
  "Version": "2012-10-17",
  "Statement": [
	{
	  "Action": ["s3:ListBucket"],
	  "Effect": "Allow",
	  "Resource": ["arn:aws:s3:::mybucket"],
	  "Condition": {"StringLike": {"s3:prefix": ["${aws:username}/*"]}}
	},
	{
	  "Action": [
		"s3:GetObject",
		"s3:PutObject"
	  ],
	  "Effect": "Allow",
	  "Resource": ["arn:aws:s3:::mybucket/${aws:username}/*"]
	}
  ]
}
```

If the user is authenticating using an STS credential which was authorized from OpenID connect we allow all `jwt:*` variables specified in the JWT specification, custom `jwt:*` or extensions are not supported. List of policy variables for OpenID based STS.

- `jwt:sub`
- `jwt:iss`
- `jwt:aud`
- `jwt:jti`
- `jwt:upn`
- `jwt:name`
- `jwt:groups`
- `jwt:given_name`
- `jwt:family_name`
- `jwt:middle_name`
- `jwt:nickname`
- `jwt:preferred_username`
- `jwt:profile`
- `jwt:picture`
- `jwt:website`
- `jwt:email`
- `jwt:gender`
- `jwt:birthdate`
- `jwt:phone_number`
- `jwt:address`
- `jwt:scope`
- `jwt:client_id`

Following example shows OpenID users with full programmatic access to a OpenID user-specific directory (their own "home directory") in Silo.

```
{
  "Version": "2012-10-17",
  "Statement": [
	{
	  "Action": ["s3:ListBucket"],
	  "Effect": "Allow",
	  "Resource": ["arn:aws:s3:::mybucket"],
	  "Condition": {"StringLike": {"s3:prefix": ["${jwt:preferred_username}/*"]}}
	},
	{
	  "Action": [
		"s3:GetObject",
		"s3:PutObject"
	  ],
	  "Effect": "Allow",
	  "Resource": ["arn:aws:s3:::mybucket/${jwt:preferred_username}/*"]
	}
  ]
}
```

If the user is authenticating using an STS credential which was authorized from AD/LDAP we allow `ldap:*` variables.

Currently supports

- `ldap:username`
- `ldap:user`
- `ldap:groups`

Following example shows LDAP users full programmatic access to a LDAP user-specific directory (their own "home directory") in Silo.

```
{
  "Version": "2012-10-17",
  "Statement": [
	{
	  "Action": ["s3:ListBucket"],
	  "Effect": "Allow",
	  "Resource": ["arn:aws:s3:::mybucket"],
	  "Condition": {"StringLike": {"s3:prefix": ["${ldap:username}/*"]}}
	},
	{
	  "Action": [
		"s3:GetObject",
		"s3:PutObject"
	  ],
	  "Effect": "Allow",
	  "Resource": ["arn:aws:s3:::mybucket/${ldap:username}/*"]
	}
  ]
}
```

#### Common information available in all requests

- `aws:CurrentTime` - This can be used for conditions that check the date and time.
- `aws:EpochTime` - This is the date in epoch or Unix time, for use with date/time conditions.
- `aws:PrincipalType` - This value indicates whether the principal is an account (Root credential), user (Silo user), or assumed role (STS)
- `aws:SecureTransport` - This is a Boolean value that represents whether the request was sent over TLS.
- `aws:SourceIp` - This is the requester's IP address, for use with IP address conditions. If running behind Nginx like proxies, Silo preserve's the source IP.

```
{
  "Version": "2012-10-17",
  "Statement": {
	"Effect": "Allow",
	"Action": "s3:ListBucket*",
	"Resource": "arn:aws:s3:::mybucket",
	"Condition": {"IpAddress": {"aws:SourceIp": "203.0.113.0/24"}}
  }
}
```

- `aws:UserAgent` - This value is a string that contains information about the requester's client application. This string is generated by the client and can be unreliable. You can only use this context key from `mc` or other compatible SDKs which standardize the User-Agent string.
- `aws:username` - This is a string containing the friendly name of the current user, this value would point to STS temporary credential in `AssumeRole`ed requests, use `jwt:preferred_username` in case of OpenID connect and `ldap:username` in case of AD/LDAP. *aws:userid* is an alias to *aws:username* in Silo.
- `aws:groups` - This is an array containing the group names, this value would point to group mappings for the user, use `jwt:groups` in case of OpenID connect and `ldap:groups` in case of AD/LDAP.

## Explore Further

- [Silo Client Complete Guide](https://silo.pgsty.com/reference/minio-mc/)
- [Silo STS Quickstart Guide](https://silo.pgsty.com/developers/security-token-service/)
- [Silo Admin Complete Guide](https://silo.pgsty.com/reference/minio-mc-admin/)
- [The Silo documentation website](https://silo.pgsty.com/docs/)
