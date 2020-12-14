package store

// Store is complex system
// Will update anytime, so should use standard to complete

type Alcuber interface {
	// attach, datech bounding to node
	// Attach is dev_connect
	// Detach is dev_disconnect
	DoConn(conf *DynConf, pool, image string) (string, error)
	DoDisConn(conf *DynConf, pool, image string) error

	// notice alcuber the node is not ready
	// because shutdown, network down, etc...
	FailNode(conf *DynConf, node string) error

	// device should be recreate after problem happen
	// should call when node recover from exception
	DevStop(conf *DynConf, pool, image string) error

	// Get all nodes in the same cluste
	// now group will only include three node
	GetNode(conf *DynConf, node string) ([]string, error)
}
