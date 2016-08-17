package gluster

import "encoding/xml"

// XXX Gluster volume structure

// Brick represents gluster brick
type Brick struct {
	Name string `xml:"name"`
	UUID string `xml:"hostUuid"`
}

// Option represents various glsuter options: nfs.enable, transport.address-family, etc.
type Option struct {
	Name  string `xml:"name"`
	Value string `xml:"value"`
}

// Volume represents gluster volume as per `gluster volume info --xml`
type Volume struct {
	Name          string   `xml:"name"`
	Type          string   `xml:"typeStr"`
	ID            string   `xml:"id"`
	Status        string   `xml:"statusStr"`
	NumBricks     int      `xml:"brickCount"`
	TransportType int      `xml:"transport"`
	Bricks        []Brick  `xml:"bricks>brick"`
	Options       []Option `xml:"options>option"`
}

// Volumes represents array of gluster volumes
type Volumes struct {
	XMLName xml.Name `xml:"cliOutput"`
	List    []Volume `xml:"volInfo>volumes>volume"`
}
