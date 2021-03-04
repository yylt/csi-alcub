package store

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"

	rbd2 "github.com/yylt/csi-alcub/pkg/rbd"
	"github.com/yylt/csi-alcub/utils"

	"github.com/imroc/req"
	klog "k8s.io/klog/v2"
)

var _ Alcuber = &client{}

const (
	resource = "dev"
	devpath  = "/dev"
)

var (
	defaultHeader = http.Header{
		"content-type": []string{"application/json"},
		"Accept":       []string{"application/json"},
	}
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

func NewClient(alcubConf *AlcubConf, dynconf *DynConf, conntimeout time.Duration) *client {
	if alcubConf == nil || alcubConf.ApiUrl == "" {
		panic("alcub configure must not be nil and apiurl must not be nil")
	}
	reqcli := req.New()
	reqcli.SetTimeout(conntimeout)
	cli := &client{
		cli:     reqcli,
		conf:    alcubConf,
		dynConf: dynconf,
	}
	if cli.dynConf != nil {
		err := cli.fillAlcubUrl(cli.dynConf)
		if err != nil {
			//panic(err)
			klog.Errorf("fetch alcuburl failed:%v", err)
		}
	}
	return cli
}

func (c *client) DoConn(conf *DynConf, pool, image string) (string, error) {
	var (
		reterr   error
		httpcode int
	)
	var devbody = struct {
		Dev string `json:"alcubierre_dev"`
	}{}
	reterr = c.do(conf, func(buf *bytes.Buffer, au http.Header, dc *DynConf) error {
		data := map[string]interface{}{
			"op": "dev_connect",
			"op_args": map[string]string{
				"pool":  pool,
				"image": image,
			},
		}
		klog.V(5).Infof("start do connect alcubierre server")
		resp, err := c.cli.Post(buf.String(), au, defaultHeader, req.BodyJSON(data))
		if err != nil {
			klog.Errorf("do connect failed:%v, data:%v", err, data)
			return err
		}

		if resp != nil && resp.Response() != nil {
			httpcode = resp.Response().StatusCode
			err = resp.ToJSON(&devbody)
			if err != nil {
				klog.Errorf("resp body toJson failed: %v", err)
			}
		}
		klog.V(2).Infof("do connect done, data:%v, code: %v, dev: %v", data, httpcode, devbody.Dev)
		return err
	})
	if reterr != nil {
		return "", reterr
	}
	if devbody.Dev == "" {
		return "", fmt.Errorf("not found device on alcubierre_dev")
	}
	return path.Join(devpath, devbody.Dev), nil
}

func (c *client) DoDisConn(conf *DynConf, pool, image string) error {
	var errbody = struct {
		Serr string `json:"error,omitempty"`
	}{}
	return c.do(conf, func(buf *bytes.Buffer, au http.Header, dc *DynConf) error {
		data := map[string]interface{}{
			"op": "dev_disconnect",
			"op_args": map[string]string{
				"pool":  pool,
				"image": image,
			},
		}
		klog.V(5).Infof("start do disconnect alcubierre server")
		resp, err := c.cli.Post(buf.String(), au, defaultHeader, req.BodyJSON(data))
		if err != nil {
			klog.Errorf("do disconnect failed: %v", err)
			return err
		}

		httpcode := resp.Response().StatusCode
		// response body maybe null, but it is successs
		resp.ToJSON(&errbody)

		klog.V(2).Infof("do disconnect done, data:%v, code:%d, resp:%v", data, httpcode, resp.String())

		if errbody.Serr != "" {
			klog.Errorf("do disconnect failed: %v", errbody.Serr)
			return errors.New(errbody.Serr)
		}

		return nil
	})
}

func (c *client) FailNode(conf *DynConf, node string) error {

	return c.do(conf, func(buf *bytes.Buffer, au http.Header, dc *DynConf) error {

		data := map[string]interface{}{
			"op": "node_fail",
			"op_args": map[string]string{
				"node": node,
			},
		}
		klog.V(5).Infof("start fail node from alcub: %v", data)
		resp, err := c.cli.Post(buf.String(), au, defaultHeader, req.BodyJSON(data))

		klog.V(2).Infof("fail node done,resp:%v err:%v", resp, err)
		if err != nil {
			return err
		}
		return nil
	})
}

func (c *client) DevStop(conf *DynConf, pool, image string) error {
	return c.do(conf, func(buf *bytes.Buffer, au http.Header, dc *DynConf) error {

		data := map[string]interface{}{
			"op": "dev_stop",
			"op_args": map[string]string{
				"pool":  pool,
				"image": image,
			},
		}
		klog.V(5).Infof("start dev stop from alcub")
		resp, err := c.cli.Post(buf.String(), au, defaultHeader, req.BodyJSON(data))

		klog.V(2).Infof("dev stop done,resp:%v err:%v", resp, err)
		if err != nil {
			return err
		}
		return nil
	})
}

// GetImageStatus
// return isclear
func (c *client) GetImageStatus(conf *DynConf, pool, image string) bool {

	var clearbody = struct {
		Status string `json:"status,omitempty"`
	}{}
	reterr := c.do(conf, func(buf *bytes.Buffer, au http.Header, dc *DynConf) error {
		data := map[string]interface{}{
			"pool":  pool,
			"image": image,
		}
		resp, err := c.cli.Get(buf.String(), au, defaultHeader, req.BodyJSON(data))
		if err != nil {
			klog.Errorf("Get image(%s) status failed:%v", image, err)
			return err
		}
		klog.V(2).Infof("Get image status done, data:%v, resp:%v", data, resp.String())
		err = resp.ToJSON(&clearbody)
		if err != nil {
			klog.Errorf("To json data failed:%v", err)
			return err
		}

		return nil
	})
	if reterr != nil {
		return false
	}
	if clearbody.Status == "" || clearbody.Status == "clean" {
		return true
	}
	return false
}

// Actually getNode fetch other nodes alcub Url
// so add local alcuburl into nodes
func (c *client) GetNode(conf *DynConf, nodename string) ([]string, error) {
	var (
		nodes  []string
		reterr error
	)
	reterr = c.do(conf, func(buf *bytes.Buffer, au http.Header, dc *DynConf) error {

		data := map[string]interface{}{
			"op": "get_secondary_urls",
			"op_args": map[string]string{
				"node": nodename,
			},
		}
		klog.V(5).Infof("start get node from alcub")
		resp, err := c.cli.Post(buf.String(), au, defaultHeader, req.BodyJSON(data))

		if err != nil {
			klog.Errorf("Get node failed:%v", err)
			return err
		}

		err = resp.ToJSON(&nodes)
		if err != nil {
			klog.Errorf("To json data faield:%v", err)
			return err
		}
		alcubu, err := url.Parse(string(dc.AlucbUrl))
		if err != nil {
			return nil
		}
		//TODO evict same node
		nodes = append(nodes, string(utils.Combine(alcubu.Scheme, "://", alcubu.Host)))
		klog.V(4).Infof("Get all node: %v", nodes)
		return nil
	})
	if reterr != nil {
		return nil, reterr
	}
	return nodes, nil
}

func (c *client) do(dynconf *DynConf, fn func(buf *bytes.Buffer, au http.Header, c *DynConf) error) error {
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
		err = c.fillAlcubUrl(dconf)
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
	buf := utils.GetBuf()
	buf.Write(utils.Combine(dst.Scheme, "://"))
	buf.WriteString(path.Join(dst.Host, c.conf.ApiUrl, resource))
	err = fn(buf, auth, dconf)
	utils.PutBuf(buf)

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
