# broker-ari

This is an experiment to host an MQTT broker for Ariston water heater devices so it is possible to manage them over LAN
(something every vendor should provide by default). It is also an API server that is compatible with the official API
(https://www.ariston-net.remotethermo.com/api/v2/).

However, it is not feature complete, not well-tested and does not support all models.

## Architecture

This software consists of three modules:

- MQTT proxy
- DNS server
- API server

An MQTT broker is listening on :8883 and accepts connections from your Ariston devices. You need to tweak your network so that
the `broker-ari.everyware-cloud.com` hostname gets resolved to the IP address where this MQTT broker of this software is listening.
You may do that by configuring static IP settings on the appliance and set the DNS resolver to the IP address where the DNS server
of this software is listening.

This software also supports relaying the MQTT messages to the official MQTT broker of the vendor back and forth. This feature is 
turned on by default. This means, if you use the official mobile app and it calls the official API (the real service), 
everything should just work smoothly.

The API server has been tested with the https://pypi.org/project/ariston/ client.

## Supported operations

- Retrieving temperatures (current/set)
- Retrieving mode (e.g. BOOST)
- Set mode
- Set temperature

## Tested appliances

- Lydos Hybrid

## Example

```
$ openssl req -nodes -x509 -sha256 -newkey rsa:2048 \
  -keyout  broker-ari.everyware-cloud.com.key \
  -out  broker-ari.everyware-cloud.com.crt \
  -days 356 \
  -subj "/C=NL/ST=Zuid Holland/L=Rotterdam/O=ACME Corp/OU=IT Dept/CN=broker-ari.everyware-cloud.com"  \
  -addext "subjectAltName = DNS:broker-ari.everyware-cloud.com" 
$ ./broker-ari --mqtt-broker-certificate-path ./broker-ari.everyware-cloud.com.crt --mqtt-broker-private-key-path ./broker-ari.everyware-cloud.com.key --api-username someuser --api-password somepass
```
