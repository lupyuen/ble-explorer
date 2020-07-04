# ble-explorer

Experiment to show phones nearby running TraceTogether.

To build and run:

```bash
# Ignore the message "go get: no install location"
go get ./...
go build main.go

# Scan for 5 seconds, connect timeout is 1 second
sudo ./main -sd 5s -cd 1s
```

Output:

```
Scanning for 5s...
Scanned 32 devices
Connecting to devices...
[xx:xx:xx:xx:xx:xx] RSSI -91:
Svcs: [b82ab3fc15954f6a80f0fe094cc218f9]
Manu: FF03613539
    Service: b82ab3fc15954f6a80f0fe094cc218f9 , Handle (0x28)
      Characteristic: b82ab3fc15954f6a80f0fe094cc218f9 , Property: 0x0A (RW), Handle(0x29), VHandle
(0x2A)
        Value         a01d79c04aa66...
      Characteristic: 117bdd5857ce4e7a8e877cccdda2a804 , Property: 0x0A (RW), Handle(0x2B), VHandle
(0x2C)
        Value         a01d79c04aa66...

Done
```

Based on

- https://github.com/JuulLabs-OSS/ble/tree/master/examples/basic/scanner

- https://github.com/JuulLabs-OSS/ble/tree/master/examples/basic/explorer
