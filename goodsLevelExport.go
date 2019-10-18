package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/linzhaopeng/goExcel"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type ResponseData struct {
	Body struct {
		Data struct {
			GoodsLevel interface{} `json:"goodsLevel"`
		} `json:"data"`
		Ret     string `json:"ret"`
		RetInfo string `json:"retinfo"`
	} `json:"body"`
}

type ExcelData struct {
	newGoodsLevel   string
	engineerOptions string
	productId       string
	preGoodsLevel   string
	seriesNumber    string
}

type EngineerOption []struct {
	Id string      `json:"id"`
	Mp interface{} `json:"mp"`
}

type DetectSnap []struct {
	Item []struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	} `json:"item"`
}

var (
	headList = map[string]int{
		"产品ID":  0,
		"条码":    1,
		"检测机况":  2,
		"销售等级":  3,
		"原销售等级": 4,
	}
	count = 0
	mu    sync.Mutex
)

func getGoodsLevel(e interface{}, p string, channelId string, seriesNumber string, preGoodsLevel string, wg *sync.WaitGroup, ch chan *ExcelData, optionStr string) {
	excelData := &ExcelData{
		seriesNumber:    seriesNumber,
		newGoodsLevel:   "",
		engineerOptions: optionStr,
		productId:       p,
		preGoodsLevel:   preGoodsLevel,
	}
	defer func(excelData *ExcelData) {
		mu.Lock()
		count++
		fmt.Println(count)
		mu.Unlock()

		ch <- excelData
		wg.Done()
	}(excelData)

	requestData := map[string]map[string]interface{}{
		"head": {
			"msgtype":   "request",
			"remark":    "",
			"version":   "0.01",
			"interface": "getDetectGoodsInfoExport",
		},
		"params": {
			"engineerOptions": e,
			"productId":       p,
			"channelId":       channelId,
			"login_user_id":   "1010",
			"login_token":     "59130f6359343072d89a570ebc269570",
			"login_system_id": "55",
			"system":          "upin",
			"time":            time.Now().Unix(),
		},
	}

	requestJson, err := json.Marshal(requestData)
	if err != nil {
		return
	}

	// fmt.Println(string(requestJson))
	res, err := http.Post("http://detect-sales-online.huishoubao.com.cn/detect/getDetectGoodsInfoExport", "application/x-www-form-urlencoded", strings.NewReader(string(requestJson)))
	if err != nil {
		return
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}

	var responseData ResponseData
	if err := json.Unmarshal(body, &responseData); err != nil {
		return
	}

	if responseData.Body.Ret != "0" {
		excelData.newGoodsLevel = responseData.Body.RetInfo
		return
	}

	goodsLevel, _ := json.Marshal(responseData.Body.Data.GoodsLevel)
	if string(goodsLevel) != "[]" {
		excelData.newGoodsLevel = string(goodsLevel)
	}
}

func main() {
	fmt.Println(time.Now().Format("2006-01-02 15:04:05"))
	file, err := os.Open("/Users/roc/Downloads/query_result.csv")
	if err != nil {
		fmt.Println("open file fail: ", err.Error())
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Println("read fail: ", err.Error())
		return
	}

	exists, recordList := make(map[string]int8), make([]map[string]string, 0)
	for i := 1; i < len(records); i ++ {
		if _, ok := exists[records[i][0]]; ok {
			continue
		}
		exists[records[i][0]] = 1

		metadata := make(map[string]string)
		for k := 0; k < len(records[0]); k ++ {
			metadata[records[0][k]] = records[i][k]
		}
		recordList = append(recordList, metadata)
	}

	var wg              sync.WaitGroup

	ch := make(chan *ExcelData)
	for k, data := range recordList {
		if data["FchannelId"] == "10000207" {
			continue
		}

		if k%10 == 9 {
			time.Sleep(time.Second)
		}

		var (
			engineerOptions EngineerOption
			e				interface{}
			detectSnap      DetectSnap
		)

		if err := json.Unmarshal([]byte(data["Fengineer_options"]), &e); err != nil {
			fmt.Println("engineerOptions decode fail: ", err.Error())
			return
		}

		if err := json.Unmarshal([]byte(data["Fengineer_options"]), &engineerOptions); err != nil {
			fmt.Println("engineerOptions decode fail: ", err.Error())
			return
		}

		if len(strings.TrimSpace(data["Fdetect_snap"])) > 0 {
			if err := json.Unmarshal([]byte(data["Fdetect_snap"]), &detectSnap); err != nil {
				//fmt.Println("detectSnap decode fail: ", err.Error(), ", string: ", data["Fdetect_snap"])
				//return
				continue
			}
		}

		optionList := make([]string, 0)
		optionNameList := make(map[string]string)

		for _, option := range detectSnap {
			for _, item := range option.Item {
				optionNameList[item.Id] = item.Name
			}
		}

		for _, engineerOption := range engineerOptions {
			if name, ok := optionNameList[engineerOption.Id]; ok {
				optionList = append(optionList, name)
			}
		}

		optionStr := strings.Join(optionList, ",")
		wg.Add(1)
		go getGoodsLevel(e, data["Fproduct_id"], data["Fchannel_id"], data["Fseries_number"], data["Fgoods_level"], &wg, ch, optionStr)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var excelData []map[string]string
	for data := range ch {
		excelData = append(excelData, map[string]string{
			"条码":    data.seriesNumber,
			"销售等级":  data.newGoodsLevel,
			"检测机况":  data.engineerOptions,
			"产品ID":  data.productId,
			"原销售等级": data.preGoodsLevel,
		})
	}
	fmt.Println(time.Now().Format("2006-01-02 15:04:05"))
	goExcel.Export(excelData, headList, "销售等级")
	fmt.Println(time.Now().Format("2006-01-02 15:04:05"))
}
