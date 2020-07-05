package main

import (
	"github.com/tidusant/c3m-common/c3mcommon"
	"github.com/tidusant/c3m-common/inflect"
	"github.com/tidusant/c3m-common/log"

	rpch "github.com/tidusant/chadmin-repo/cuahang"
	"github.com/tidusant/chadmin-repo/models"
	"gopkg.in/mgo.v2/bson"

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
	log.Debugf("request data: %s", data)
	*result = c3mcommon.ReturnJsonMessage("0", "No action found.", "", "")
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

	//	if usex.Action == "c" {
	//		*result = CreateProduct(usex)

	//check shop permission
	shop := rpch.GetShopById(usex.UserID, ShopID)
	if shop.Status == 0 {
		*result = c3mcommon.ReturnJsonMessage("-4", "Shop is disabled.", "", "")
		return nil
	}
	usex.Shop = shop

	//	} else
	if usex.Action == "s" {
		*result = SaveNews(usex)
	} else if usex.Action == "l" {
		*result = LoadNews(usex)
	} else if usex.Action == "la" {
		*result = LoadAllNews(usex)
	} else if usex.Action == "r" {
		*result = RemoveNews(usex)
	} else if usex.Action == "sc" {
		*result = SaveCat(usex)
	} else if usex.Action == "lc" {
		*result = LoadCat(usex)
	} else if usex.Action == "rc" {
		*result = RemoveCat(usex)
	}

	return nil
}

func LoadCat(usex models.UserSession) string {
	log.Debugf("loadcat begin")
	cats := rpch.GetAllNewsCats(usex.UserID, usex.Shop.ID.Hex())

	for i, _ := range cats {
		cats[i].ShopId = ""
		cats[i].UserId = ""
	}
	b, _ := json.Marshal(cats)
	return c3mcommon.ReturnJsonMessage("1", "", "success", string(b))
}
func SaveCat(usex models.UserSession) string {
	var cat models.NewsCat
	err := json.Unmarshal([]byte(usex.Params), &cat)
	if !c3mcommon.CheckError("create cat parse json", err) {
		return c3mcommon.ReturnJsonMessage("0", "create catalog fail", "", "")
	}
	olditem := cat
	isnewitem := true
	//get all item
	items := rpch.GetAllNewsCats(usex.UserID, usex.Shop.ID.Hex())
	for _, item := range items {
		if item.ID == cat.ID {
			isnewitem = false
			olditem = item
			break
		}
	}
	//check max cat limited
	if isnewitem {

		// if rpch.GetShopLimitbyKey(usex.Shop.ID.Hex(), "maxnewscat") <= len(items) {
		// 	return c3mcommon.ReturnJsonMessage("0", "max news category limit reach", "", "")
		// }
	}
	//get all slug
	// slugs := rpch.GetAllSlug(usex.UserID, usex.Shop.ID.Hex())
	// mapslugs := make(map[string]string)
	// for i := 0; i < len(slugs); i++ {
	// 	mapslugs[slugs[i]] = slugs[i]
	// }

	//slug
	langslugs := make(map[string]models.Slug)
	var langlinks []models.LangLink
	for lang, _ := range cat.Langs {
		var newslug models.Slug
		newslug.ShopId = usex.Shop.ID.Hex()
		newslug.Object = "newscat"
		newslug.Lang = lang
		newslug.TemplateCode = usex.Shop.Theme

		if cat.Langs[lang].Name == "" {
			//check if oldprod has value, else delete
			if olditem.Langs[lang] == nil {
				delete(cat.Langs, lang)
			} else {
				//not update for null lang
				if cat.Langs[lang].Description != "" {
					cat.Langs[lang] = olditem.Langs[lang]
				} else {
					//delete old lang if all info is blank
					newslug.ObjectId = olditem.ID.Hex()
					rpch.RemoveSlug(cat.Langs[lang].Slug, usex.Shop.ID.Hex())
					delete(cat.Langs, lang)
				}
			}
			continue
		}
		//newslug

		newslug.Slug = inflect.Parameterize(cat.Langs[lang].Name)
		langslugs[lang] = newslug
	}
	//check code duplicate
	if isnewitem {

		cat.ShopId = usex.Shop.ID.Hex()
		cat.UserId = usex.UserID
		cat.Created = time.Now().UTC().Add(time.Hour + 7)
		cat.ID = bson.NewObjectId()
	} else {
		//update field here:
		olditem.Langs = cat.Langs
		olditem.Publish = cat.Publish
		olditem.Feature = cat.Feature
		olditem.Home = cat.Home
		olditem.Avatar = cat.Avatar

		cat = olditem
	}
	//update langlink
	for lang, slug := range langslugs {
		slug.ObjectId = olditem.ID.Hex()
		olditem.Langs[lang].Slug = rpch.SaveSlugNoBuild(slug)
		slug.Slug = olditem.Langs[lang].Slug
		langlinks = append(langlinks, models.LangLink{Href: olditem.Langs[lang].Slug + "/", Code: lang, Name: c3mcommon.GetLangnameByCode(lang)})
		langslugs[lang] = slug

	}
	cat.LangLinks = langlinks

	strrt := rpch.SaveNewsCat(&cat)
	if strrt == "" {
		return c3mcommon.ReturnJsonMessage("0", "error", "error", "")
	}

	//rebuild
	b, err := json.Marshal(cat)
	//create build
	strrt = string(b)
	errstr := rpch.CreateBuild("newscat", cat.ID.Hex(), strrt, usex)
	if errstr != "" {
		return c3mcommon.ReturnJsonMessage("0", errstr, "build error", "")
	}
	errstr = rpch.CreateCommonDataBuild(usex)
	if errstr != "" {
		return c3mcommon.ReturnJsonMessage("0", errstr, "build error", "")
	}
	//rpb.CreateBuild(build)

	//build home
	// var bs models.BuildScript
	// shop := rpch.GetShopById(usex.UserID, usex.Shop.ID.Hex())
	// bs.ShopID = usex.Shop.ID.Hex()
	// bs.TemplateCode = shop.Theme
	// bs.Domain = shop.Domain
	// bs.ObjectID = "home"
	// rpb.CreateBuild(bs)

	// //build cat
	// bs.Collection = "newscats"
	// bs.ObjectID = cat.Code
	// rpb.CreateBuild(bs)

	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)
}
func RemoveCat(usex models.UserSession) models.RequestResult {

	catid := usex.Params
	//check product

	news := rpch.GetNewsByCatId(usex.UserID, usex.Shop.ID.Hex(), catid)
	subcats := rpch.GetSubCatsByID(usex.Shop.ID.Hex(), catid)
	if len(news) > 0 || len(subcats) > 0 {
		return c3mcommon.ReturnJsonMessage("0", "Catalog not empty, Please remove all subcategory and news.", "", "")
	}

	cat := rpch.GetNewsCatByID(usex.Shop.ID.Hex(), catid)
	for lang, _ := range cat.Langs {
		//remove slug
		var newslug models.Slug
		newslug.ShopId = usex.Shop.ID.Hex()
		newslug.Object = "newscat"
		newslug.Lang = lang

		newslug.ObjectId = cat.ID.Hex()

		rpch.RemoveSlug(cat.Langs[lang].Slug)
		delete(cat.Langs, lang)

	}
	rpch.SaveNewsCat(&cat)

	return c3mcommon.ReturnJsonMessage("1", "", "success", "")

}

func LoadNews(usex models.UserSession) models.RequestResult {
	args := strings.Split(usex.Params, ",")
	if len(args) < 1 {
		return c3mcommon.ReturnJsonMessage("0", "error submit fields", "", "")
	}

	newsid := args[0]
	item := rpch.GetNewsByID(usex.UserID, usex.Shop.ID.Hex(), newsid)
	b, _ := json.Marshal(item)

	return c3mcommon.ReturnJsonMessage("1", "", "", string(b))

}
func LoadAllNews(usex models.UserSession) models.RequestResult {

	items := rpch.GetAllNews(usex.Shop.ID.Hex())
	if len(items) == 0 {
		return c3mcommon.ReturnJsonMessage("0", "no news found", "", "")
	}
	//get all cat
	cats := rpch.GetAllNewsCats(usex.Shop.ID.Hex())
	catmap := make(map[string]models.NewsCat)
	for _, item := range cats {
		catmap[item.ID.Hex()] = item
	}
	for i, item := range items {
		for _, lang := range usex.Shop.Config.Langs {
			//check news lang exist
			if _, ok := item.Langs[lang]; ok {
				if len(item.CatIDs) > 0 {
					var cat []string
					for _, catid := range item.CatIDs {
						if catmap[catid].ID.Hex() != "" {
							//check cat lang exist
							if val, ok := catmap[catid].Langs[lang]; ok {
								cat = append(cat, val.Title)
							}
						}
					}
					items[i].Langs[lang].Catname = strings.Join(cat, "<br />")

				} else {
					items[i].Langs[lang].Catname = "Root"
				}
			}
		}
		items[i].ShopID = ""
		items[i].UserID = ""
	}
	b, _ := json.Marshal(items)
	return c3mcommon.ReturnJsonMessage("1", "", "success", string(b))

}
func SaveNews(usex models.UserSession) models.RequestResult {
	var newitem models.News
	log.Debugf("Unmarshal %s", usex.Params)
	err := json.Unmarshal([]byte(usex.Params), &newitem)
	if !c3mcommon.CheckError("json parse news", err) {
		return c3mcommon.ReturnJsonMessage("0", "json parse fail", "", "")
	}

	isnewitem := true
	var olditem models.News
	//get all item
	items := rpch.GetAllNews(usex.Shop.ID.Hex())
	for _, item := range items {
		if item.ID == newitem.ID {
			isnewitem = false
			olditem = item
			break
		}
	}
	//check max cat limited
	if isnewitem {
		if rpch.GetShopLimitbyKey(usex.Shop.ID.Hex(), "maxnews") <= len(items) {
			return c3mcommon.ReturnJsonMessage("3", "error", "max news limit reach", "")
		}
	}

	//get all slug
	// slugs := rpch.GetAllSlug(usex.UserID, usex.Shop.ID.Hex())
	// mapslugs := make(map[string]string)
	// for i := 0; i < len(slugs); i++ {
	// 	mapslugs[slugs[i]] = slugs[i]
	// }
	//get array of album slug
	// allcodes := map[string]string{}
	// for _, item := range items {
	// 	allcodes[item.Code] = item.Code
	// 	if !isnewitem && item.Code == newitem.Code {
	// 		olditem = item
	// 	}
	// }

	//slug
	langslugs := make(map[string]models.Slug)
	var langlinks []models.LangLink
	for lang, _ := range newitem.Langs {
		var newslug models.Slug
		newslug.ShopId = usex.Shop.ID.Hex()
		newslug.Object = "news"
		newslug.Lang = lang

		if newitem.Langs[lang].Title == "" {
			delete(newitem.Langs, lang)
			continue
		}
		if newitem.Langs[lang].Title == "" {
			//check if oldprod has value, else delete
			if olditem.Langs[lang] == nil {
				delete(newitem.Langs, lang)
			} else {
				//not update for null lang
				if newitem.Langs[lang].Description != "" {
					newitem.Langs[lang] = olditem.Langs[lang]
				} else {
					//delete old lang if all info is blank
					newslug.ObjectId = olditem.ID.Hex()
					rpch.RemoveSlug(newitem.Langs[lang].Slug)
					delete(newitem.Langs, lang)
				}
			}
			continue
		}
		//newslug
		//newslug
		newslug.Slug = inflect.Parameterize(newitem.Langs[lang].Title)

		langslugs[lang] = newslug

	}

	//check code duplicate
	if isnewitem {
		//insert new
		// newcode := ""
		// for {
		// 	newcode = mystring.RandString(3)
		// 	if _, ok := allcodes[newcode]; !ok {
		// 		break
		// 	}
		// }
		// newitem.Code = newcode
		newitem.ShopID = usex.Shop.ID.Hex()
		newitem.UserID = usex.UserID
		newitem.Created = time.Now().UTC()
		newitem.ID = bson.NewObjectId()
	} else {
		//update field here:
		olditem.Langs = newitem.Langs
		olditem.CatIDs = newitem.CatIDs
		olditem.Publish = newitem.Publish
		olditem.Feature = newitem.Feature
		olditem.Home = newitem.Home
		olditem.Avatar = newitem.Avatar
		olditem.Modified = time.Now().UTC()
		newitem = olditem
	}

	//update langlinks
	for lang, slug := range langslugs {
		slug.ObjectId = newitem.ID.Hex()

		newitem.Langs[lang].Slug = rpch.SaveSlugNoBuild(slug)
		slug.Slug = newitem.Langs[lang].Slug
		langlinks = append(langlinks, models.LangLink{Href: newitem.Langs[lang].Slug + "/", Code: lang, Name: c3mcommon.GetLangnameByCode(lang)})
		langslugs[lang] = slug

	}
	newitem.LangLinks = langlinks
	strrt := rpch.SaveNews(&newitem)
	if strrt == "0" {
		return c3mcommon.ReturnJsonMessage("0", "error", "error", "")
	}
	log.Debugf("savenews %s", strrt)
	//rebuild page
	newitem.ShopID = ""
	newitem.UserID = ""
	b, err := json.Marshal(newitem)
	strrt = string(b)
	if newitem.Publish || newitem.Home || newitem.Feature {
		rpch.CreateBuild("news", newitem.ID.Hex(), strrt, usex)
	}
	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)
}

func RemoveNews(usex models.UserSession) models.RequestResult {

	newsids := strings.Split(usex.Params, ",")
	for _, newsid := range newsids {
		itemremove := rpch.GetNewsByID(usex.UserID, usex.Shop.ID.Hex(), newsid)
		//remove slug
		for _, lang := range itemremove.Langs {
			rpch.RemoveSlug(lang.Slug)
		}
		rpch.RemoveNews(itemremove)
		b, _ := json.Marshal(itemremove)

		if itemremove.Publish || itemremove.Home || itemremove.Feature {
			rpch.CreateBuild("remove", itemremove.ID.Hex(), string(b), usex)
		}
	}
	return c3mcommon.ReturnJsonMessage("1", "", "success", "")

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

//repush 3
