package store

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sync"

	rbd2 "github.com/yylt/csi-alcub/pkg/rbd"
	"github.com/yylt/csi-alcub/utils"

	"github.com/imroc/req"
	klog "k8s.io/klog/v2"
)

var _ Alcuber = &client{}

var (
	resource = "dev"
	devpath  = "/dev"
)

var (
	defaultHeader = http.Header{
		"content-type": []string{"application/json"},
		"Accept":       []string{"application/json"},
	}
	bufpool = sync.Pool{New: func() interface{} {
		return new(bytes.Buffer)
	}}
)

type AlcubConf struct {
	AlucbPool string
	ApiUrl    string
	User      string
	Password  string
}

// dynamic configure
type DynConf struct {
	//set by function FinishAlcubUrl()
	AlucbUrl []byte

	Nodename string
}

type client struct {
	cli  *req.Req
	conf *AlcubConf

	dynConf *DynConf
}

func NewClient(alcubConf *AlcubConf, dynconf *DynConf) *client {
	if alcubConf == nil || alcubConf.ApiUrl == "" {
		panic("alcub configure must not be nil and apiurl must not be nil")
	}
	cli := &client{
		cli:     req.New(),
		conf:    alcubConf,
		dynConf: dynconf,
	}
	if cli.dynConf != nil {
		err := cli.fillAlcubUrl(cli.dynConf)
		if err != nil {
			panic(err)
		}
	}
	return cli
}

func (c *client) DoConn(conf *DynConf, pool, image string) (string, error) {
	var (
		reterr error
	)
	var devbody = struct {
		Dev string `json:"alcubierre_dev"`
	}{}
	reterr = c.do(conf, func(buf *bytes.Buffer, au http.Header) error {

		data := map[string]interface{}{
			"op": "dev_connect",
			"op_args": map[string]string{
				"pool":  pool,
				"image": image,
			},
		}
		resp, err := c.cli.Post(buf.String(), au, defaultHeader, req.BodyJSON(data))

		klog.V(4).Infof("do connect done,resp:%v err:%v", resp, err)
		if err != nil {
			return err
		}
		return resp.ToJSON(&devbody)
	})
	if reterr != nil {
		return "", reterr
	}

	return path.Join(devpath, devbody.Dev), nil
}

func (c *client) DoDisConn(conf *DynConf, pool, image string) error {

	return c.do(conf, func(buf *bytes.Buffer, au http.Header) error {

		data := map[string]interface{}{
			"op": "dev_disconnect",
			"op_args": map[string]string{
				"pool":  pool,
				"image": image,
			},
		}
		resp, err := c.cli.Post(buf.String(), au, defaultHeader, req.BodyJSON(data))

		klog.V(4).Infof("do disconnect done,resp:%v err:%v", resp, err)
		if err != nil {
			return err
		}
		return nil
	})
}

func (c *client) FailNode(conf *DynConf, node string) error {

	return c.do(conf, func(buf *bytes.Buffer, au http.Header) error {

		data := map[string]interface{}{
			"op": "node_fail",
			"op_args": map[string]string{
				"node": node,
			},
		}
		resp, err := c.cli.Post(buf.String(), au, defaultHeader, req.BodyJSON(data))

		klog.V(4).Infof("fail node done,resp:%v err:%v", resp, err)
		if err != nil {
			return err
		}
		return nil
	})
}

func (c *client) DevStop(conf *DynConf, pool, image string) error {
	return c.do(conf, func(buf *bytes.Buffer, au http.Header) error {

		data := map[string]interface{}{
			"op": "dev_stop",
			"op_args": map[string]string{
				"pool":  pool,
				"image": image,
			},
		}
		resp, err := c.cli.Post(buf.String(), au, defaultHeader, req.BodyJSON(data))

		klog.V(4).Infof("dev stop done,resp:%v err:%v", resp, err)
		if err != nil {
			return err
		}
		return nil
	})
}

func (c *client) GetNode(conf *DynConf, node string) ([]string, error) {
	var (
		nodes  []string
		reterr error
	)
	reterr = c.do(conf, func(buf *bytes.Buffer, au http.Header) error {

		data := map[string]interface{}{
			"op": "get_secondary_urls",
			"op_args": map[string]string{
				"node": node,
			},
		}

		resp, err := c.cli.Post(buf.String(), au, defaultHeader, req.BodyJSON(data))

		klog.V(4).Infof("Get node done,resp:%v err:%v", resp, err)
		if err != nil {
			return err
		}
		return resp.ToJSON(&nodes)
	})
	if reterr != nil {
		return nil, reterr
	}
	return nodes, nil
}

func (c *client) do(dynconf *DynConf, fn func(buf *bytes.Buffer, au http.Header) error) error {
	var (
		auth  http.Header
		err   error
		dconf *DynConf
	)
	if dynconf == nil && c.dynConf == nil {
		return fmt.Errorf("No dynmic Configure found")
	}
	if dynconf != nil {
		dconf = dynconf
	} else {
		dconf = c.dynConf
	}

	if len(dconf.AlucbUrl) == 0 {
		err = c.fillAlcubUrl(dynconf)
		if err != nil {
			return err
		}
	}
	if c.conf.User != "" {
		auth = utils.BuildBasicAuthMd5([]byte(c.conf.User), []byte(c.conf.Password))
	}
	dst, err := url.Parse(string(dconf.AlucbUrl))
	if err != nil {
		return err
	}
	buf := bufpool.Get().(*bytes.Buffer)
	buf.Reset()
	//TODO: optimise
	buf.WriteString(fmt.Sprintf("%s://", dst.Scheme))
	buf.WriteString(path.Join(dst.Host, c.conf.ApiUrl, resource))

	err = fn(buf, auth)
	bufpool.Put(buf)

	return err
}

func (c *client) fillAlcubUrl(dynconf *DynConf) error {
	attr := fmt.Sprintf("alcubierre_node_%s", dynconf.Nodename)
	alcuburl, err := rbd2.FetchUrl(c.conf.AlucbPool, attr)
	klog.V(2).Infof("fetch alcub-url: url %s, err:%v", alcuburl, err)
	if err != nil {
		return err
	}
	dynconf.AlucbUrl = alcuburl
	return nil
}
