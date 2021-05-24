// Copyright 2015 The go-orange Authors
// This file is part of the go-orange library.
//
// The go-orange library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-orange library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-orange library. If not, see <http://www.gnu.org/licenses/>.

/*

Package rpc implements bi-directional JSON-RPC 2.0 on multiple transports.

It provides access to the exported Methods of an object across a network or other I/O
connection. After creating a server or client instance, objects can be registered to make
them visible as 'services'. Exported Methods that follow specific conventions can be
called remotely. It also has support for the publish/subscribe pattern.

RPC Methods

Methods that satisfy the following criteria are made available for remote access:

 - Method must be exported
 - Method returns 0, 1 (response or error) or 2 (response and error) values

An example Method:

 func (s *CalcService) Add(a, b int) (int, error)

When the returned error isn't nil the returned integer is ignored and the error is sent
back to the client. Otherwise the returned integer is sent back to the client.

Optional arguments are supported by accepting pointer values as arguments. E.g. if we want
to do the addition in an optional finite field we can accept a mod argument as pointer
value.

 func (s *CalcService) Add(a, b int, mod *int) (int, error)

This RPC Method can be called with 2 integers and a null value as third argument. In that
case the mod argument will be nil. Or it can be called with 3 integers, in that case mod
will be pointing to the given third argument. Since the optional argument is the last
argument the RPC package will also accept 2 integers as arguments. It will pass the mod
argument as nil to the RPC Method.

The server offers the ServeCodec Method which accepts a ServerCodec instance. It will read
requests from the codec, process the request and sends the response back to the client
using the codec. The server can execute requests concurrently. Responses can be sent back
to the client out of order.

An example server which uses the JSON codec:

 type CalculatorService struct {}

 func (s *CalculatorService) Add(a, b int) int {
	return a + b
 }

 func (s *CalculatorService) Div(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("divide by zero")
	}
	return a/b, nil
 }

 calculator := new(CalculatorService)
 server := NewServer()
 server.RegisterName("calculator", calculator)
 l, _ := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: "/tmp/calculator.sock"})
 server.ServeListener(l)

Subscriptions

The package also supports the publish subscribe pattern through the use of subscriptions.
A Method that is considered eligible for notifications must satisfy the following
criteria:

 - Method must be exported
 - first Method argument type must be context.Context
 - Method must have return types (rpc.Subscription, error)

An example Method:

 func (s *BlockChainService) NewBlocks(ctx context.Context) (rpc.Subscription, error) {
 	...
 }

When the service containing the subscription Method is registered to the server, for
example under the "blockchain" namespace, a subscription is created by calling the
"blockchain_subscribe" Method.

Subscriptions are deleted when the user sends an unsubscribe request or when the
connection which was used to create the subscription is closed. This can be initiated by
the client and server. The server will close the connection for any write error.

For more information about subscriptions, see https://github.com/ong2020/go-orange/wiki/RPC-PUB-SUB.

Reverse Calls

In any Method handler, an instance of rpc.Client can be accessed through the
ClientFromContext Method. Using this client instance, server-to-client Method calls can be
performed on the RPC connection.
*/
package rpc
