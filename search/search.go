package search

import (
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/notegio/openrelay/blockhash"
	"github.com/notegio/openrelay/common"
	dbModule "github.com/notegio/openrelay/db"
	// "github.com/notegio/openrelay/types"
	"net/http"
	urlModule "net/url"
	"strconv"
	"strings"
)

func FormatResponse(orders []dbModule.Order, format string, total, page, perPage int) ([]byte, string, error) {
	if format == "application/octet-stream" {
		result := []byte{}
		for _, order := range orders {
			orderBytes := order.Bytes()
			result = append(result, orderBytes[:]...)
		}
		return result, "application/octet-stream", nil
	} else {
		orderList := []FormattedOrder{}
		for _, order := range orders {
			orderList = append(orderList, *GetFormattedOrder(&order))
		}
		result, err := json.Marshal(GetPagedResult(total, page, perPage, orderList))
		return result, "application/json", err
	}
}

func FormatSingleResponse(order *dbModule.Order, format string) ([]byte, string, error) {
	if format == "application/octet-stream" {
		result := order.Bytes()
		return result[:], "application/octet-stream", nil
	}
	result, err := json.Marshal(order)
	return result, "application/json", err
}

func applyFilter(query *gorm.DB, queryField, dbField string, queryObject urlModule.Values) (*gorm.DB, error) {
	if address := queryObject.Get(queryField); address != "" {
		addressBytes, err := common.HexToBytes(address)
		if err != nil {
			return query, err
		}
		whereClause := fmt.Sprintf("%v = ?", dbField)
		filteredQuery := query.Where(whereClause, common.BytesToOrAddress(addressBytes))
		return filteredQuery, filteredQuery.Error
	}
	return query, nil
}

func applyOrFilter(query *gorm.DB, queryField, dbField1, dbField2 string, queryObject urlModule.Values) (*gorm.DB, error) {
	if address := queryObject.Get(queryField); address != "" {
		addressBytes, err := common.HexToBytes(address)
		if err != nil {
			return query, err
		}
		whereClause := fmt.Sprintf("%v = ? or %v = ?", dbField1, dbField2)
		filteredQuery := query.Where(whereClause, common.BytesToOrAddress(addressBytes), common.BytesToOrAddress(addressBytes))
		return filteredQuery, filteredQuery.Error
	}
	return query, nil
}

func returnError(w http.ResponseWriter, err error, code int) {
	w.WriteHeader(code)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf("{\"code\":100,\"reason\":\"%v\"}", err.Error())))
}

func returnErrorList(w http.ResponseWriter, errs []ValidationError) {
	w.WriteHeader(400)
	apiError := ApiError{100, "Validation Failed", errs}
	w.Header().Set("Content-Type", "application/json")
	data, err := json.Marshal(apiError)
	if err == nil {
		w.Write(data)
	} else {
		w.Write([]byte(err.Error()))
	}

}

func getPages(queryObject urlModule.Values) (int, int, error) {
	pageStr := queryObject.Get("page")
	if pageStr == "" {
		pageStr = "1"
	}
	perPageStr := queryObject.Get("per_page")
	if perPageStr == "" {
		perPageStr = "20"
	}
	pageInt, err := strconv.Atoi(pageStr)
	if err != nil {
		return 0, 0, err
	}
	perPageInt, err := strconv.Atoi(perPageStr)
	if err != nil {
		return 0, 0, err
	}
	return pageInt, perPageInt, nil
}

func BlockHashDecorator(blockHash blockhash.BlockHash, fn func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	blockHash.Get() // Start the go routines, if necessary
	return func(w http.ResponseWriter, r *http.Request) {
		queryObject := r.URL.Query()
		hash := queryObject.Get("blockhash")
		if hash == "" {
			queryObject.Set("blockhash", strings.Trim(blockHash.Get(), "\""))
			url := *r.URL
			url.RawQuery = queryObject.Encode()
			w.Header().Set("Cache-Control", "max-age=5, public")
			http.Redirect(w, r, (&url).RequestURI(), 307)
			return
		}
		fn(w, r)
	}
}

func filterByNetworkId(query *gorm.DB, queryObject urlModule.Values, exchangeLookup *dbModule.ExchangeLookup) (*gorm.DB, error) {
	networkID, err := strconv.Atoi(queryObject.Get("networkId"))
	if err != nil {
		networkID = 1
	}
	exchanges, err := exchangeLookup.GetExchangesByNetwork(int64(networkID))
	if err != nil {
		return query, err
	}
	if len(exchanges) == 0 {
		return query, fmt.Errorf("Network id %v is not supported", networkID)
	}
	queryStrings := []string{}
	for _, _ = range exchanges {
		queryStrings = append(queryStrings, "exchange_address = ?")
	}
	// Note that while we are using string manipulation to build the query, the
	// only thing the user can provide is the network number. The list of
	// exchanges can be considered sanitized data, and even then that only
	// impacts the length of the query string - the actual addresses are
	// parameterized.
	query = query.Where(fmt.Sprintf("(%v)", strings.Join(queryStrings, " OR ")), exchanges)
	return query, nil
}

func SearchHandler(db *gorm.DB) func(http.ResponseWriter, *http.Request) {
	exchangeLookup := dbModule.NewExchangeLookup(db)
	return func(w http.ResponseWriter, r *http.Request) {
		queryObject := r.URL.Query()
		query := db.Model(&dbModule.Order{}).Where("status = ?", dbModule.StatusOpen)

		errs := []ValidationError{}

		query, err := filterByNetworkId(query, queryObject, exchangeLookup)

		if err != nil {
			errs = append(errs, ValidationError{err.Error(), 1006, "networkId"})
		}


		query, err = applyFilter(query, "exchangeContractAddress", "exchange_address", queryObject)
		if err != nil {
			errs = append(errs, ValidationError{err.Error(), 1003, "exchangeContractAddress"})
		}
		query, err = applyFilter(query, "makerAssetAddress", "maker_asset_address", queryObject)
		if err != nil {
			errs = append(errs, ValidationError{err.Error(), 1003, "makerAssetAddress"})
		}
		query, err = applyFilter(query, "takerAssetAddress", "taker_asset_address", queryObject)
		if err != nil {
			errs = append(errs, ValidationError{err.Error(), 1003, "takerAssetAddress"})
		}
		query, err = applyFilter(query, "makerAssetData", "maker_asset_data", queryObject)
		if err != nil {
			errs = append(errs, ValidationError{err.Error(), 1003, "makerAssetData"})
		}
		query, err = applyFilter(query, "takerAssetData", "taker_asset_data", queryObject)
		if err != nil {
			errs = append(errs, ValidationError{err.Error(), 1003, "takerAssetData"})
		}
		query, err = applyFilter(query, "maker", "maker", queryObject)
		if err != nil {
			errs = append(errs, ValidationError{err.Error(), 1003, "maker"})
		}
		query, err = applyFilter(query, "taker", "taker", queryObject)
		if err != nil {
			errs = append(errs, ValidationError{err.Error(), 1003, "taker"})
		}
		query, err = applyFilter(query, "feeRecipient", "fee_recipient", queryObject)
		if err != nil {
			errs = append(errs, ValidationError{err.Error(), 1003, "feeRecipient"})
		}
		query, err = applyOrFilter(query, "assetAddress", "maker_asset_address", "taker_asset_address", queryObject)
		if err != nil {
			errs = append(errs, ValidationError{err.Error(), 1003, "assetAddress"})
		}
		query, err = applyOrFilter(query, "assetData", "maker_address", "taker_address", queryObject)
		if err != nil {
			errs = append(errs, ValidationError{err.Error(), 1001, "assetData"})
		}
		query, err = applyOrFilter(query, "trader", "maker", "taker", queryObject)
		if err != nil {
			errs = append(errs, ValidationError{err.Error(), 1003, "trader"})
		}

		pageInt, perPageInt, err := getPages(queryObject)
		if err != nil {
			errs = append(errs, ValidationError{err.Error(), 1001, "page"})
		}
		query = query.Where("expiration_timestamp_in_sec > ?", getExpTime(queryObject))
		var count int
		query.Count(&count)
		query = query.Offset((pageInt - 1) * perPageInt).Limit(perPageInt)
		if query.Error != nil {
			errs = append(errs, ValidationError{err.Error(), 1001, "_expTime"})
		}
		if len(errs) > 0 {
			returnErrorList(w, errs)
			return
		}

		if queryObject.Get("makerAssetAddress") != "" && queryObject.Get("takerAssetAddress") != "" {
			query := query.Order("price asc, fee_rate asc")
			if query.Error != nil {
				returnError(w, query.Error, 500)
				return
			}
		} else {
			query := query.Order("updated_at")
			if query.Error != nil {
				returnError(w, query.Error, 500)
				return
			}
		}

		orders := []dbModule.Order{}
		if count > (pageInt - 1) * perPageInt {
			if err := query.Find(&orders).Error; err != nil {
				returnError(w, err, 500)
				return
			}
		}
		var acceptHeader string
		if acceptVal, ok := r.Header["Accept"]; ok {
			acceptHeader = strings.Split(acceptVal[0], ";")[0]
		} else {
			acceptHeader = "unknown"
		}
		response, contentType, err := FormatResponse(orders, acceptHeader, count, pageInt, perPageInt)
		if err == nil {
			w.WriteHeader(200)


			url := *r.URL
			queryObject.Set("page", strconv.Itoa(pageInt + 1))
			url.RawQuery = queryObject.Encode()

			w.Header().Set("Link", fmt.Sprintf("<%v>; rel=\"next\"", (&url).RequestURI()))
			w.Header().Set("Content-Type", contentType)
			w.Write(response)
		} else {
			returnError(w, err, 500)
		}
	}
}
