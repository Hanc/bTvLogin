package Scan

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"image/png"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type PollResponseData struct {
	TokenInfo    json.RawMessage `json:"token_info"`
	CookieInfo   json.RawMessage `json:"cookie_info"`
	ExpiresIn    int             `json:"expires_in"`
	RefreshToken string          `json:"refresh_token"`
	AccessKey    string          `json:"access_key"`
	Mid          int64           `json:"mid"`
	SessData     string          `json:"sess_data"`
}

// 构造请求参数签名，类似 Python 中的 tvsign 函数
func buildParams(urlStr string, params map[string]string) string {
	u, _ := url.Parse(urlStr)
	query := u.Query()
	for k, v := range params {
		query.Set(k, v)
	}
	u.RawQuery = query.Encode()

	return u.String()
}

func QRLogin() {
	result := getTvUrl()
	// 生成二维码
	qrcode := result["data"].(map[string]interface{})["url"].(string)
	authCode := result["data"].(map[string]interface{})["auth_code"].(string)
	generateQRCode(qrcode)
	poll(authCode)
}

func getTvUrl() map[string]interface{} {
	appkey := "4409e2ce8ffd12b8"
	appsec := "59b43e04ad6965f34319062b478f83dd"
	local_id := "0"
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	params := tvsign(map[string]string{
		"local_id": local_id,
		"ts":       ts,
	}, appkey, appsec)
	apiUrl := "https://passport.bilibili.com/x/passport-tv-login/qrcode/auth_code"
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}
	resp, err := http.PostForm(apiUrl, values)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	defer resp.Body.Close()

	// 处理响应数据
	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	fmt.Println(result)
	return result
}

func tvsign(params map[string]string, appkey string, appsec string) map[string]string {
	// 将 appkey 加入参数中
	params["appkey"] = appkey

	// 重排序参数 key
	keys := make([]string, len(params))
	i := 0
	for k := range params {
		keys[i] = k
		i++
	}
	sort.Strings(keys)

	// 序列化参数
	var paramsList []string
	for _, key := range keys {
		value := params[key]
		paramsList = append(paramsList, fmt.Sprintf("%s=%s", key, value))
	}
	query := strings.Join(paramsList, "&")

	// 计算 api 签名
	h := md5.New()
	h.Write([]byte(query + appsec))
	sign := hex.EncodeToString(h.Sum(nil))

	// 将签名加入参数中并返回
	params["sign"] = sign
	return params
}

func generateQRCode(content string) {
	// Create the barcode
	qrCode, _ := qr.Encode(content, qr.M, qr.Auto)

	// Scale the barcode to 200x200 pixels
	qrCode, _ = barcode.Scale(qrCode, 200, 200)

	// create the output file
	file, _ := os.Create("b_qrcode.png")
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {

		}
	}(file)

	// encode the barcode as png
	err := png.Encode(file, qrCode)
	if err != nil {
		return
	}
}

func poll(authCode string) {
	for {
		// 轮询二维码状态
		defalut_params := map[string]string{
			"auth_code": authCode,
			"local_id":  "0",
			"ts":        strconv.FormatInt(time.Now().Unix(), 10),
		}
		params := tvsign(defalut_params, "4409e2ce8ffd12b8", "59b43e04ad6965f34319062b478f83dd")
		pollUrl := buildParams("https://passport.bilibili.com/x/passport-tv-login/qrcode/poll", params)
		resp, err := http.PostForm(pollUrl, url.Values{})
		if err != nil {
			panic(err)
		}

		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {

			}
		}(resp.Body)
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}

		fmt.Println(string(body))

		var pollInfo struct {
			Code    int               `json:"code"`
			Message string            `json:"message"`
			Data    *PollResponseData `json:"data,omitempty"`
		}
		if err := json.Unmarshal(body, &pollInfo); err != nil {
			panic(err)
		}
		switch pollInfo.Code {
		case 0:
			// 登录成功，保存登录信息
			saveInfo := map[string]interface{}{
				"update_time": int(time.Now().Unix()),
				"token_info":  pollInfo.Data.TokenInfo,
				"cookie_info": pollInfo.Data.CookieInfo,
			}
			data, err := json.MarshalIndent(saveInfo, "", "    ")
			if err != nil {
				panic(err)
			}

			if err := ioutil.WriteFile("b_info.json", data, 0644); err != nil {
				panic(err)
			}

			fmt.Printf("登录成功，有效期至%s\n", time.Unix(int64(time.Now().Unix()+int64(pollInfo.Data.ExpiresIn)), 0).Format("2006-01-02 15:04:05"))
			return

		case -3:
			fmt.Println("API校验密匙错误")
			continue

		case -400:
			fmt.Println("请求错误")
			continue

		case 86038:
			fmt.Println("二维码已失效")
			continue

		case 86039:
			// 二维码未过期，等待5秒后再次轮询
			time.Sleep(5 * time.Second)
		}
	}
}
