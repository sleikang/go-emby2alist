package emby

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"github.com/AmbitiousJun/go-emby2alist/internal/util/colors"
	"github.com/AmbitiousJun/go-emby2alist/internal/util/jsons"
	"github.com/gin-gonic/gin"
)

// HandleSyncDownload 处理 Sync 下载接口, 重定向到直链
func HandleSyncDownload(c *gin.Context) {
	// 解析出 JobItems id
	itemInfo, err := resolveItemInfo(c)
	if checkErr(c, err) {
		return
	}
	log.Printf(colors.ToBlue("解析出来的 itemInfo 信息: %v"), jsons.NewByVal(itemInfo))
	if itemInfo.Id == "" {
		checkErr(c, errors.New("JobItems id 为空"))
		return
	}

	// 请求 targets 列表
	targetUri := "/Sync/Targets?api_key=" + itemInfo.ApiKey
	resp, _ := Fetch(targetUri, http.MethodGet, nil, nil)
	if resp.Code != http.StatusOK {
		checkErr(c, fmt.Errorf("请求 emby 失败: %v, uri: %s", resp.Msg, targetUri))
		return
	}
	targets := resp.Data
	if targets.Empty() {
		checkErr(c, fmt.Errorf("targets 列表为空, 原始响应: %v", targets))
		return
	}

	// 每个 id 逐一尝试
	readyUriTmpl := "/Sync/Items/Ready?api_key=" + itemInfo.ApiKey + "&TargetId="
	targets.RangeArr(func(_ int, target *jsons.Item) error {
		id, ok := target.Attr("Id").String()
		if !ok {
			return nil
		}

		// 请求 Ready 接口
		readyUri := readyUriTmpl + id
		resp, _ := Fetch(readyUri, http.MethodGet, nil, nil)
		if resp.Code != http.StatusOK {
			checkErr(c, fmt.Errorf("请求 emby 失败: %v, uri: %s", resp.Msg, readyUri))
			return jsons.ErrBreakRange
		}
		readyItems := resp.Data
		if readyItems.Empty() {
			return nil
		}

		// 遍历所有 item
		breakRange := false
		readyItems.RangeArr(func(_ int, ri *jsons.Item) error {
			jobId, ok := ri.Attr("SyncJobItemId").Int()
			if !ok {
				return nil
			}
			if strconv.Itoa(jobId) != itemInfo.Id {
				return nil
			}

			// 匹配成功, 获取到下载项目的 ItemId, 重新封装请求, 进行直链重定向
			itemId, ok := ri.Attr("Item").Attr("Id").String()
			if !ok {
				checkErr(c, fmt.Errorf("解析 emby 响应异常: 获取不到 itemId, 原始响应: %v", ri))
				breakRange = true
				return jsons.ErrBreakRange
			}
			msId, ok := ri.Attr("Item").Attr("MediaSources").Idx(0).Attr("Id").String()
			if !ok {
				checkErr(c, fmt.Errorf("解析 emby 响应异常: 获取不到 mediaSourceId, 原始响应: %v", ri))
				breakRange = true
				return jsons.ErrBreakRange
			}
			log.Printf(colors.ToGreen("成功匹配到 itemId: %s, mediaSourceId: %s"), itemId, msId)

			newUrl, _ := url.Parse(fmt.Sprintf("/videos/%s/stream?MediaSourceId=%s&api_key=%s&Static=true", itemId, msId, itemInfo.ApiKey))
			c.Redirect(http.StatusTemporaryRedirect, newUrl.String())
			breakRange = true
			return jsons.ErrBreakRange
		})

		if breakRange {
			return jsons.ErrBreakRange
		}

		return nil
	})

}
