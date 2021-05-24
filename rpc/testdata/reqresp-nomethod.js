// This test calls a Method that doesn't exist.

--> {"jsonrpc": "2.0", "id": 2, "Method": "invalid_Method", "params": [2, 3]}
<-- {"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"the Method invalid_Method does not exist/is not available"}}
