package main

import (
	"github.com/tidusant/c3m-common/c3mcommon"
	"github.com/tidusant/c3m-common/inflect"
	"github.com/tidusant/c3m-common/log"
	"github.com/tidusant/c3m-common/lzjs"
	"github.com/tidusant/c3m-common/mystring"
	rpb "github.com/tidusant/chadmin-repo/builder"
	rpch "github.com/tidusant/chadmin-repo/cuahang"
	"github.com/tidusant/chadmin-repo/models"
	//	"c3m/common/inflect"
	//	"c3m/log"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"strconv"
	"strings"
	"time"
)

const (
	defaultcampaigncode string = "XVsdAZGVmY"
)

type Arith int

func (t *Arith) Run(data string, result *string) error {
	log.Debugf("Call RPCprod args:" + data)
	*result = ""
	//parse args
	args := strings.Split(data, "|")

	if len(args) < 3 {
		return nil
	}
	var usex models.UserSession
	usex.Session = args[0]
	usex.Action = args[2]
	info := strings.Split(args[1], "[+]")
	usex.UserID = info[0]
	ShopID := info[1]
	usex.Params = ""
	if len(args) > 3 {
		usex.Params = args[3]
	}

	//check shop permission
	shop := rpch.GetShopById(usex.UserID, ShopID)
	if shop.Status == 0 {
		*result = c3mcommon.ReturnJsonMessage("-4", "Shop is disabled.", "", "")
		return nil
	}
	usex.Shop = shop
	if usex.Action == "s" {
		*result = SaveNews(usex)
	} else if usex.Action == "l" {
		*result = LoadNews(usex)
	} else if usex.Action == "la" {
		*result = LoadAllNews(usex)
	} else if usex.Action == "r" {
		*result = Remove(usex)
	} else if usex.Action == "sc" {
		*result = SaveCat(usex)
	} else if usex.Action == "lc" {
		*result = LoadCat(usex)
	} else if usex.Action == "rc" {
		*result = RemoveCat(usex)
	} else { //default
		*result = ""
	}

	return nil
}

func LoadCat(usex models.UserSession) string {
	log.Debugf("loadcat begin")
	cats := rpch.GetAllNewsCats(usex.UserID, usex.Shop.ID.Hex())
	strrt := "["
	catinfstr := ""
	for _, cat := range cats {
		catlangs := ""
		for lang, catinf := range cat.Langs {
			catlangs += "\"" + lang + "\":{\"Name\":\"" + catinf.Name + "\",\"Slug\":\"" + catinf.Slug + "\"},"
		}
		catlangs = catlangs[:len(catlangs)-1]
		catinfstr += "{\"Code\":\"" + cat.Code + "\",\"Langs\":{" + catlangs + "}},"
	}
	if catinfstr == "" {
		strrt += "{}]"
	} else {
		strrt += catinfstr[:len(catinfstr)-1] + "]"
	}
	log.Debugf("loadcat %s", strrt)
	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)
}
func SaveCat(usex models.UserSession) string {
	var cat models.NewsCat
	err := json.Unmarshal([]byte(usex.Params), &cat)
	if !c3mcommon.CheckError("create cat parse json", err) {
		return c3mcommon.ReturnJsonMessage("0", "create catalog fail", "", "")
	}
	olditem := cat
	newcat := false
	if cat.Code == "" {
		newcat = true
	}

	//get all cats
	cats := rpch.GetAllNewsCats(usex.UserID, usex.Shop.ID.Hex())
	//check max cat limited
	if newcat {
		shop := rpch.GetShopById(usex.UserID, usex.Shop.ID.Hex())
		if shop.Config.MaxCat <= len(cats) {
			return c3mcommon.ReturnJsonMessage("3", "error", "max limit reach", "")
		}
	}
	//get all slug
	slugs := rpch.GetAllSlug(usex.UserID, usex.Shop.ID.Hex())
	mapslugs := make(map[string]string)
	for i := 0; i < len(slugs); i++ {
		mapslugs[slugs[i]] = slugs[i]
	}
	//get array of code
	catcodes := make(map[string]string)
	//get old item
	for _, c := range cats {
		catcodes[c.Code] = c.Code
		if !newcat && c.Code == cat.Code {
			olditem = c
		}
	}

	for lang, _ := range cat.Langs {
		if cat.Langs[lang].Name == "" {
			delete(cat.Langs, lang)
			continue
		}
		//newslug
		tb, _ := lzjs.DecompressFromBase64(cat.Langs[lang].Name)
		newslug := inflect.Parameterize(string(tb))
		cat.Langs[lang].Slug = newslug

		isChangeSlug := true
		if !newcat {
			if olditem.Langs[lang].Slug == newslug {
				isChangeSlug = false
			}
		}

		if isChangeSlug {
			//check slug duplicate
			i := 1
			for {
				if _, ok := mapslugs[cat.Langs[lang].Slug]; ok {
					cat.Langs[lang].Slug = newslug + strconv.Itoa(i)
					i++
				} else {
					mapslugs[cat.Langs[lang].Slug] = cat.Langs[lang].Slug
					break
				}
			}
			//remove oldslug
			if !newcat {
				rpch.RemoveSlug(olditem.Langs[lang].Slug, usex.Shop.ID.Hex())
			}
			rpch.CreateSlug(cat.Langs[lang].Slug, usex.Shop.ID.Hex(), "prodcats")
		}
	}

	//check code duplicate
	if newcat {
		//insert new
		newcode := ""
		for {
			newcode = mystring.RandString(3)
			if _, ok := catcodes[newcode]; !ok {
				break
			}
		}
		cat.Code = newcode
		cat.ShopId = usex.Shop.ID.Hex()
		cat.UserId = usex.UserID
		cat.Created = time.Now().UTC().Add(time.Hour + 7)
	} else {
		//update
		olditem.Langs = cat.Langs
		cat = olditem
	}
	strrt := rpch.SaveNewsCat(cat)
	if strrt == "0" {
		return c3mcommon.ReturnJsonMessage("0", "error", "error", "")
	}
	log.Debugf("saveprod %s", strrt)
	//build home
	var bs models.BuildScript
	shop := rpch.GetShopById(usex.UserID, usex.Shop.ID.Hex())
	bs.ShopID = usex.Shop.ID.Hex()
	bs.TemplateCode = shop.Theme
	bs.Domain = shop.Domain
	bs.ObjectID = "home"
	rpb.CreateBuild(bs)

	//build cat
	bs.Collection = "newscats"
	bs.ObjectID = cat.Code
	rpb.CreateBuild(bs)

	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)
}
func RemoveCat(usex models.UserSession) string {

	args := strings.Split(usex.Params, ",")
	if len(args) < 1 {
		return c3mcommon.ReturnJsonMessage("0", "error submit fields", "", "")
	}
	log.Debugf("remove cat %s", args)
	code := args[0]
	if code == "unc" {
		return c3mcommon.ReturnJsonMessage("0", "error cannot delete cat", "", "")
	}
	//check product

	news := rpch.GetNewsByCatId(usex.UserID, usex.Shop.ID.Hex(), code)

	if len(news) > 0 {
		return c3mcommon.ReturnJsonMessage("2", "Catalog not empty", "", "")
	}

	cat := rpch.GetNewsCatByCode(usex.UserID, usex.Shop.ID.Hex(), code)
	for lang, _ := range cat.Langs {
		//remove slug
		rpch.RemoveSlug(cat.Langs[lang].Slug, usex.Shop.ID.Hex())
		delete(cat.Langs, lang)
	}
	rpch.SaveNewsCat(cat)

	//build home
	var bs models.BuildScript
	shop := rpch.GetShopById(usex.UserID, usex.Shop.ID.Hex())
	bs.ShopID = usex.Shop.ID.Hex()
	bs.TemplateCode = shop.Theme
	bs.Domain = shop.Domain
	bs.ObjectID = "home"
	rpb.CreateBuild(bs)

	//build cat
	bs.Collection = "rmnewscats"
	bs.ObjectID = cat.Code
	rpb.CreateBuild(bs)

	return c3mcommon.ReturnJsonMessage("1", "", "success", "")

}

func Remove(usex models.UserSession) string {
	log.Debugf("remove  %s", usex.Params)
	args := strings.Split(usex.Params, ",")
	if len(args) < 2 {
		return c3mcommon.ReturnJsonMessage("0", "error submit fields", "", "")
	}
	log.Debugf("save prod %s", args)
	code := args[0]
	lang := args[1]
	itemremove := rpch.GetNewsByCode(usex.UserID, usex.Shop.ID.Hex(), code)
	if itemremove.Langs[lang] != nil {
		//remove slug
		rpch.RemoveSlug(itemremove.Langs[lang].Slug, usex.Shop.ID.Hex())
		delete(itemremove.Langs, lang)
		rpch.SaveNews(itemremove)
	}

	//build home
	var bs models.BuildScript
	shop := rpch.GetShopById(usex.UserID, usex.Shop.ID.Hex())
	bs.ShopID = usex.Shop.ID.Hex()
	bs.TemplateCode = shop.Theme
	bs.Domain = shop.Domain
	bs.ObjectID = "home"
	rpb.CreateBuild(bs)

	//build cat
	bs.Collection = "news"
	bs.ObjectID = itemremove.Code
	rpb.CreateBuild(bs)
	return c3mcommon.ReturnJsonMessage("1", "", "success", "")

}
func LoadNews(usex models.UserSession) string {
	args := strings.Split(usex.Params, ",")
	if len(args) < 1 {
		return c3mcommon.ReturnJsonMessage("0", "error submit fields", "", "")
	}

	code := args[0]
	item := rpch.GetNewsByCode(usex.UserID, usex.Shop.ID.Hex(), code)
	info, _ := json.Marshal(item.Langs)
	strrt := "{\"Code\":\"" + item.Code + "\",\"CatID\":\"" + item.CatID + "\",\"Langs\":" + string(info) + "}"
	log.Debugf("load news %s", strrt)
	return strrt

}
func LoadAllNews(usex models.UserSession) string {

	items := rpch.GetAllNews(usex.UserID, usex.Shop.ID.Hex())
	if len(items) == 0 {
		return c3mcommon.ReturnJsonMessage("2", "", "no news found", "")
	}

	strrt := "["

	for _, item := range items {
		strlang := "{"
		for lang, langinfo := range item.Langs {
			langinfo.Content = ""
			info, _ := json.Marshal(langinfo)
			strlang += "\"" + lang + "\":" + string(info) + ","
		}
		strlang = strlang[:len(strlang)-1] + "}"
		strrt += "{\"Code\":\"" + item.Code + "\",\"CatID\":\"" + item.CatID + "\",\"Langs\":" + strlang + "},"
	}
	strrt = strrt[:len(strrt)-1] + "]"
	log.Debugf("loadprod %s", strrt)
	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)

}
func SaveNews(usex models.UserSession) string {
	var newitem models.News
	log.Debugf("Unmarshal %s", usex.Params)
	err := json.Unmarshal([]byte(usex.Params), &newitem)
	if !c3mcommon.CheckError("json parse news", err) {
		return c3mcommon.ReturnJsonMessage("0", "json parse fail", "", "")
	}

	isnewitem := false
	if newitem.Code == "" {
		isnewitem = true
	}
	//get all item
	items := rpch.GetAllNews(usex.UserID, usex.Shop.ID.Hex())
	var olditem models.News
	//check max cat limited
	if isnewitem {
		shop := rpch.GetShopById(usex.UserID, usex.Shop.ID.Hex())
		if shop.Config.MaxNews <= len(items) {
			return c3mcommon.ReturnJsonMessage("3", "error", "max limit reach", "")
		}
	}

	//get all slug
	slugs := rpch.GetAllSlug(usex.UserID, usex.Shop.ID.Hex())
	mapslugs := make(map[string]string)
	for i := 0; i < len(slugs); i++ {
		mapslugs[slugs[i]] = slugs[i]
	}
	//get array of album slug
	allcodes := map[string]string{}
	for _, item := range items {
		allcodes[item.Code] = item.Code
		if !isnewitem && item.Code == newitem.Code {
			olditem = item
		}
	}

	for lang, _ := range newitem.Langs {
		if newitem.Langs[lang].Title == "" {
			delete(newitem.Langs, lang)
			continue
		}
		//newslug
		//newslug
		tb, _ := lzjs.DecompressFromBase64(newitem.Langs[lang].Title)
		newslug := inflect.Parameterize(string(tb))
		newitem.Langs[lang].Slug = newslug
		isChangeSlug := true
		if !isnewitem {
			if olditem.Langs[lang].Slug == newslug {
				isChangeSlug = false
			}
		}

		if isChangeSlug {
			//check slug duplicate
			i := 1
			for {
				if _, ok := mapslugs[newitem.Langs[lang].Slug]; ok {
					newitem.Langs[lang].Slug = newslug + strconv.Itoa(i)
					i++
				} else {
					mapslugs[newitem.Langs[lang].Slug] = newitem.Langs[lang].Slug
					break
				}
			}
			//remove oldslug
			if !isnewitem {
				rpch.RemoveSlug(olditem.Langs[lang].Slug, usex.Shop.ID.Hex())
			}
			rpch.CreateSlug(newitem.Langs[lang].Slug, usex.Shop.ID.Hex(), "prodcats")
		}
	}

	//check code duplicate
	if isnewitem {
		//insert new
		newcode := ""
		for {
			newcode = mystring.RandString(3)
			if _, ok := allcodes[newcode]; !ok {
				break
			}
		}
		newitem.Code = newcode
		newitem.ShopID = usex.Shop.ID.Hex()
		newitem.UserID = usex.UserID
		newitem.Created = time.Now().UTC().Add(time.Hour + 7)
	} else {
		//update
		olditem.Langs = newitem.Langs
		olditem.CatID = newitem.CatID
		newitem = olditem
	}
	if newitem.CatID == "" {
		newitem.CatID = "unc"
	}
	strrt := rpch.SaveNews(newitem)
	if strrt == "0" {
		return c3mcommon.ReturnJsonMessage("0", "error", "error", "")
	}
	log.Debugf("saveprod %s", strrt)
	//build home
	var bs models.BuildScript
	shop := rpch.GetShopById(usex.UserID, usex.Shop.ID.Hex())
	bs.ShopID = usex.Shop.ID.Hex()
	bs.TemplateCode = shop.Theme
	bs.Domain = shop.Domain
	bs.ObjectID = "home"
	rpb.CreateBuild(bs)

	//build cat
	bs.Collection = "news"
	bs.ObjectID = newitem.Code
	rpb.CreateBuild(bs)
	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)
}

func main() {
	var port int
	var debug bool
	flag.IntVar(&port, "port", 9881, "help message for flagname")
	flag.BoolVar(&debug, "debug", false, "Indicates if debug messages should be printed in log files")
	flag.Parse()

	logLevel := log.DebugLevel
	if !debug {
		logLevel = log.InfoLevel

	}

	log.SetOutputFile(fmt.Sprintf("adminDash-"+strconv.Itoa(port)), logLevel)
	defer log.CloseOutputFile()
	log.RedirectStdOut()

	//init db
	arith := new(Arith)
	rpc.Register(arith)
	log.Infof("running with port:" + strconv.Itoa(port))

	tcpAddr, err := net.ResolveTCPAddr("tcp", ":"+strconv.Itoa(port))
	c3mcommon.CheckError("rpc dail:", err)

	listener, err := net.ListenTCP("tcp", tcpAddr)
	c3mcommon.CheckError("rpc init listen", err)

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go rpc.ServeConn(conn)
	}
}
