package store

import (
	"fmt"
	rbd2 "github.com/yylt/csi-alcub/pkg/rbd"
	"github.com/yylt/csi-alcub/utils"
	"net"
	"net/http"
	"path"

	"github.com/imroc/req"
	klog "k8s.io/klog/v2"
)

var _ Alcuber = &client{}

const (
	resource = "dev"
)

var (
	defaultHeader = http.Header{
		"content-type":[]string{"application/json"},
		"Accept":[]string{"application/json"},
	}
)

type AlcubConf struct {
	ApiUrl string
	User	string
	Password string
}

// dynamic configure
type DynConf struct {
	AlucbPool 	string
	Nodename string

	AlucbUrl []byte
}


type client struct {
	cli 	*req.Req
	conf *AlcubConf

	dynConf *DynConf
}

func NewClient(alcubConf *AlcubConf, dynconf *DynConf) *client {
	if alcubConf==nil || alcubConf.ApiUrl==""{
		panic("alcub configure must not be nil and apiurl must not be nil")
	}
	 cli := &client{
	 	cli:req.New(),
	 	conf: alcubConf,
	 	dynConf: dynconf,
	 }
	 if cli.dynConf!=nil{
	 	err := getAlcubUrl(cli.dynConf)
	 	if err!= nil {
	 		panic(err)
		}
	 }
	 return cli
}


func (c *client) DoConn(conf *DynConf,pool,image string) (string,error){
	var (
		reterr error
	)
	var devbody = struct {
		Dev  string   `json:"alcubierre_dev"`
	}{}
	reterr = c.do(conf, func(baseurl string, au http.Header) error {
		url := path.Join(baseurl,resource)
		data := map[string]interface{}{
			"op": "dev_connect",
			"op_args": map[string]string{
				"pool": pool,
				"image": image,
			},
		}
		resp, err := c.cli.Post(url,au,defaultHeader,req.BodyJSON(data))

		klog.V(4).Infof("do connect done,resp:%v err:%v",resp, err)
		if err!= nil{
			return err
		}
		return resp.ToJSON(&devbody)
	})
	if reterr!= nil {
		return "",reterr
	}
	return devbody.Dev,nil
}

func (c *client) DoDisConn(conf *DynConf,pool,image string) (error){

	return c.do(conf, func(baseurl string, au http.Header) error {
		url := path.Join(baseurl,resource)
		data := map[string]interface{}{
			"op": "dev_disconnect",
			"op_args": map[string]string{
				"pool": pool,
				"image": image,
			},
		}
		resp, err := c.cli.Post(url,au,defaultHeader,req.BodyJSON(data))

		klog.V(4).Infof("do disconnect done,resp:%v err:%v",resp, err)
		if err!= nil{
			return err
		}
		return nil
	})
}

func (c *client) FailNode(conf *DynConf,node string) (error){

	return c.do(conf, func(baseurl string, au http.Header) error {
		url := path.Join(baseurl,resource)
		data := map[string]interface{}{
			"op": "node_fail",
			"op_args": map[string]string{
				"node": node,
			},
		}
		resp, err := c.cli.Post(url,au,defaultHeader,req.BodyJSON(data))

		klog.V(4).Infof("fail node done,resp:%v err:%v",resp, err)
		if err!= nil{
			return err
		}
		return nil
	})
}

func (c *client) DevStop(conf *DynConf,pool,image string) error{
	return c.do(conf, func(baseurl string, au http.Header) error {
		url := path.Join(baseurl,resource)
		data := map[string]interface{}{
			"op": "dev_stop",
			"op_args": map[string]string{
				"pool": pool,
				"image": image,
			},
		}
		resp, err := c.cli.Post(url,au,defaultHeader,req.BodyJSON(data))

		klog.V(4).Infof("dev stop done,resp:%v err:%v",resp, err)
		if err!= nil{
			return err
		}
		return nil
	})
}

//TODO Wait to impl
func (c *client) GetNode(conf *DynConf,node string) ([]string,error){
	var (
		nodes []string
		reterr error
	)
	reterr = c.do(conf, func(baseurl string, au http.Header) error {
		url := path.Join(baseurl,resource)
		data := map[string]interface{}{
			"op": "get_secondary_urls",
			"op_args": map[string]string{
				"node": node,
			},
		}
		resp, err := c.cli.Post(url,au,defaultHeader,req.BodyJSON(data))

		klog.V(4).Infof("Get node done,resp:%v err:%v",resp, err)
		if err!= nil{
			return err
		}
		//TODO parse resp to nodes!
		return nil
	})
	if reterr!=nil {
		return nil,reterr
	}
	return nodes,nil
}


func (c *client) do(dynconf *DynConf,fn func(baseurl string, au http.Header) error) ( error) {
	var (
		auth http.Header
		err error
		dconf *DynConf
	)
	if dynconf==nil && c.dynConf == nil {
		return fmt.Errorf("No dynmic Configure found")
	}
	if dynconf!=nil {
		err = getAlcubUrl(dynconf)
		if err!= nil {
			return err
		}
		dconf= dynconf
	}else{
		dconf = c.dynConf
	}
	if len(dconf.AlucbUrl)==0 {
		return fmt.Errorf("alcub url not define!")
	}
	if c.conf.User!=""{
		auth= utils.BuildBasicAuthMd5([]byte(c.conf.User),[]byte(c.conf.Password))
	}
	return fn(path.Join(string(dconf.AlucbUrl),c.conf.ApiUrl),auth)
}


func getAlcubUrl(conf *DynConf) (error){
	attr := fmt.Sprintf("alcubierre_node_%s",conf.Nodename)
	alcuburl, err := rbd2.FetchUrl(conf.AlucbPool,attr)
	klog.V(2).Infof("fetch alcub url: url %v, err:%v",alcuburl, err)
	if err!= nil {
		return err
	}
	conf.AlucbUrl = alcuburl
	return nil
}

func GetDynConf(storageip net.IP,nodename string) (*DynConf, error){
	dync := &DynConf{
		AlucbPool: fmt.Sprintf("%s:0/0",storageip),
		Nodename:  nodename,
		AlucbUrl:  nil,
	}
	err := getAlcubUrl(dync)
	if err!= nil {
		return nil,err
	}
	return dync,nil
}