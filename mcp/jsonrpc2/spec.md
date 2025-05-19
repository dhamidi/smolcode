# JSON-RPC 2.0 Specification

## Overview

JSON-RPC is a stateless, light-weight remote procedure call (RPC) protocol. This specification defines several data structures and the rules around their processing. It is transport agnostic and can be used within the same process, over sockets, over HTTP, or in many various message passing environments. It uses [JSON](http://www.json.org) ([RFC 4627](http://www.ietf.org/rfc/rfc4627.txt)) as data format.

**It is designed to be simple!**

## Conventions

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in [RFC 2119](http://www.ietf.org/rfc/rfc2119.txt).

## Terminology

- **JSON**: JavaScript Object Notation as defined in [RFC 4627](http://www.ietf.org/rfc/rfc4627.txt)
- **Primitive**: Any of the four primitive JSON types (String, Number, Boolean, and Null)
- **Structured**: Either of the structured JSON types (Object and Array)
- **Client**: The origin of Request objects and the handler of Response objects
- **Server**: The origin of Response objects and the handler of Request objects

All member names exchanged between the Client and the Server that are considered for matching of any kind should be considered to be case-sensitive. The terms function, method, and procedure can be assumed to be interchangeable.

## Type System

JSON-RPC utilizes JSON's type system:

- **Primitive types**: String, Number, Boolean, Null
- **Structured types**: Object, Array

## Version Compatibility

JSON-RPC 2.0 Request objects and Response objects may not work with existing JSON-RPC 1.0 clients or servers. However, it is easy to distinguish between the two versions as 2.0 always has a member named "jsonrpc" with a String value of "2.0" whereas 1.0 does not. Most 2.0 implementations SHOULD consider trying to handle 1.0 objects, even if not the peer-to-peer and class hinting aspects of 1.0.

## Request Object

A rpc call is represented by sending a Request object to a Server. The Request object has the following members:

| Member  | Type             | Description                                                                                                                                                                                                                                                     |
| ------- | ---------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| jsonrpc | String           | A String specifying the version of the JSON-RPC protocol. MUST be exactly "2.0".                                                                                                                                                                                |
| method  | String           | A String containing the name of the method to be invoked. Method names that begin with "rpc." are reserved for system extensions and MUST NOT be used for anything else.                                                                                        |
| params  | Array or Object  | A Structured value that holds the parameter values to be used during the invocation of the method. This member MAY be omitted.                                                                                                                                  |
| id      | String or Number | An identifier established by the Client that MUST contain a String, Number, or NULL value if included. If it is not included it is assumed to be a notification. The value SHOULD normally not be Null [1] and Numbers SHOULD NOT contain fractional parts [2]. |

**Notes**:

- [1] The use of Null as a value for the id member in a Request object is discouraged, because this specification uses a value of Null for Responses with an unknown id. Also, because JSON-RPC 1.0 uses an id value of Null for Notifications this could cause confusion in handling.
- [2] Fractional parts may be problematic, since many decimal fractions cannot be represented exactly as binary fractions.

## Notification

A Notification is a Request object without an "id" member. A Request object that is a Notification signifies the Client's lack of interest in the corresponding Response object, and as such no Response object needs to be returned to the client. The Server MUST NOT reply to a Notification, including those that are within a batch request.

Notifications are not confirmable by definition, since they do not have a Response object to be returned. As such, the Client would not be aware of any errors (like e.g. "Invalid params", "Internal error").

## Parameters

If present, parameters for the RPC call MUST be provided as a Structured value. Either by-position through an Array or by-name through an Object.

## Response Object

When an RPC call is made, the Server MUST reply with a Response, except for in the case of Notifications. The Response is expressed as a single JSON Object, with the following members:

| Member  | Type                    | Description                                                                                                                                                                                                            |
| ------- | ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| jsonrpc | String                  | A String specifying the version of the JSON-RPC protocol. MUST be exactly "2.0".                                                                                                                                       |
| result  | Any                     | This member is REQUIRED on success. This member MUST NOT exist if there was an error invoking the method. The value of this member is determined by the method invoked on the Server.                                  |
| error   | Object                  | This member is REQUIRED on error. This member MUST NOT exist if there was no error triggered during invocation. The value for this member MUST be an Object as defined in section 5.1.                                 |
| id      | String, Number, or Null | This member is REQUIRED. It MUST be the same as the value of the id member in the Request Object. If there was an error in detecting the id in the Request object (e.g. Parse error/Invalid Request), it MUST be Null. |

Either the result member or error member MUST be included, but both members MUST NOT be included.

## Error Object

When an RPC call encounters an error, the Response Object MUST contain the error member with a value that is an Object with the following members:

| Member  | Type                    | Description                                                                                                                                                                                                         |
| ------- | ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| code    | Number                  | A Number that indicates the error type that occurred.                                                                                                                                                               |
| message | String                  | A String providing a short description of the error. The message SHOULD be limited to a concise single sentence.                                                                                                    |
| data    | Primitive or Structured | A Primitive or Structured value that contains additional information about the error. This may be omitted. The value of this member is defined by the Server (e.g. detailed error information, nested errors etc.). |

### Error Codes

The error codes from and including -32768 to -32000 are reserved for pre-defined errors. Any code within this range, but not defined explicitly below is reserved for future use.

| Code             | Message          | Meaning                                                                                               |
| ---------------- | ---------------- | ----------------------------------------------------------------------------------------------------- |
| -32700           | Parse error      | Invalid JSON was received by the server. An error occurred on the server while parsing the JSON text. |
| -32600           | Invalid Request  | The JSON sent is not a valid Request object.                                                          |
| -32601           | Method not found | The method does not exist / is not available.                                                         |
| -32602           | Invalid params   | Invalid method parameter(s).                                                                          |
| -32603           | Internal error   | Internal JSON-RPC error.                                                                              |
| -32000 to -32099 | Server error     | Reserved for implementation-defined server-errors.                                                    |

The remainder of the space is available for application defined errors.

## Batch

To send several Request objects at the same time, the Client MAY send an Array filled with Request objects.

The Server should respond with an Array containing the corresponding Response objects, after all of the batch Request objects have been processed. A Response object SHOULD exist for each Request object, except that there SHOULD NOT be any Response objects for notifications. The Server MAY process a batch RPC call as a set of concurrent tasks, processing them in any order and with any width of parallelism.

The Response objects being returned from a batch call MAY be returned in any order within the Array. The Client SHOULD match contexts between the set of Request objects and the resulting set of Response objects based on the id member within each Object.

If the batch RPC call itself fails to be recognized as a valid JSON or as an Array with at least one value, the response from the Server MUST be a single Response object. If there are no Response objects contained within the Response array as it is to be sent to the client, the server MUST NOT return an empty Array and should return nothing at all.

## Examples

### RPC Call with Positional Parameters

```
--> {"jsonrpc": "2.0", "method": "subtract", "params": [42, 23], "id": 1}
<-- {"jsonrpc": "2.0", "result": 19, "id": 1}

--> {"jsonrpc": "2.0", "method": "subtract", "params": [23, 42], "id": 2}
<-- {"jsonrpc": "2.0", "result": -19, "id": 2}
```

### RPC Call with Named Parameters

```
--> {"jsonrpc": "2.0", "method": "subtract", "params": {"subtrahend": 23, "minuend": 42}, "id": 3}
<-- {"jsonrpc": "2.0", "result": 19, "id": 3}

--> {"jsonrpc": "2.0", "method": "subtract", "params": {"minuend": 42, "subtrahend": 23}, "id": 4}
<-- {"jsonrpc": "2.0", "result": 19, "id": 4}
```

### Notification

```
--> {"jsonrpc": "2.0", "method": "update", "params": [1,2,3,4,5]}
--> {"jsonrpc": "2.0", "method": "foobar"}
```

_Note: No response is sent back for notifications_

### RPC Call of Non-existent Method

```
--> {"jsonrpc": "2.0", "method": "foobar", "id": "1"}
<-- {"jsonrpc": "2.0", "error": {"code": -32601, "message": "Method not found"}, "id": "1"}
```

### RPC Call with Invalid JSON

```
--> {"jsonrpc": "2.0", "method": "foobar, "params": "bar", "baz]
<-- {"jsonrpc": "2.0", "error": {"code": -32700, "message": "Parse error"}, "id": null}
```

### RPC Call with Invalid Request Object

```
--> {"jsonrpc": "2.0", "method": 1, "params": "bar"}
<-- {"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid Request"}, "id": null}
```

### RPC Call Batch, Invalid JSON

```
--> [{"jsonrpc": "2.0", "method": "sum", "params": [1,2,4], "id": "1"}, {"jsonrpc": "2.0", "method"]
<-- {"jsonrpc": "2.0", "error": {"code": -32700, "message": "Parse error"}, "id": null}
```

### Empty Array

```
--> []
<-- {"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid Request"}, "id": null}
```

### Invalid Batch, Non-empty Array but Invalid Elements

```
--> [1]
<-- [{"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid Request"}, "id": null}]
```

### Invalid Batch with Multiple Invalid Elements

```
--> [1,2,3]
<-- [
  {"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid Request"}, "id": null},
  {"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid Request"}, "id": null},
  {"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid Request"}, "id": null}
]
```

### RPC Call Batch

```
--> [
  {"jsonrpc": "2.0", "method": "sum", "params": [1,2,4], "id": "1"},
  {"jsonrpc": "2.0", "method": "notify_hello", "params": [7]},
  {"jsonrpc": "2.0", "method": "subtract", "params": [42,23], "id": "2"},
  {"foo": "boo"},
  {"jsonrpc": "2.0", "method": "foo.get", "params": {"name": "myself"}, "id": "5"},
  {"jsonrpc": "2.0", "method": "get_data", "id": "9"}
]
<-- [
  {"jsonrpc": "2.0", "result": 7, "id": "1"},
  {"jsonrpc": "2.0", "result": 19, "id": "2"},
  {"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid Request"}, "id": null},
  {"jsonrpc": "2.0", "error": {"code": -32601, "message": "Method not found"}, "id": "5"},
  {"jsonrpc": "2.0", "result": ["hello", 5], "id": "9"}
]
```

### RPC Call Batch (all notifications)

```
--> [
  {"jsonrpc": "2.0", "method": "notify_sum", "params": [1,2,4]},
  {"jsonrpc": "2.0", "method": "notify_hello", "params": [7]}
]
<-- // Nothing is returned for all notification batches
```

## Extensions

Method names that begin with `rpc.` are reserved for system extensions, and MUST NOT be used for anything else. Each system extension is defined in a related specification. All system extensions are OPTIONAL.

## Copyright

Copyright (C) 2007-2010 by the JSON-RPC Working Group

This document and translations of it may be used to implement JSON-RPC, it may be copied and furnished to others, and derivative works that comment on or otherwise explain it or assist in its implementation may be prepared, copied, published and distributed, in whole or in part, without restriction of any kind, provided that the above copyright notice and this paragraph are included on all such copies and derivative works. However, this document itself may not be modified in any way.

The limited permissions granted above are perpetual and will not be revoked.

This document and the information contained herein is provided "AS IS" and ALL WARRANTIES, EXPRESS OR IMPLIED are DISCLAIMED, INCLUDING BUT NOT LIMITED TO ANY WARRANTY THAT THE USE OF THE INFORMATION HEREIN WILL NOT INFRINGE ANY RIGHTS OR ANY IMPLIED WARRANTIES OF MERCHANTABILITY OR FITNESS FOR A PARTICULAR PURPOSE.
