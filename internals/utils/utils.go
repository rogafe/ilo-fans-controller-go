package utils

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/big"
	"net"
	"net/http"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt"
)

var (
	Assets map[string]string = map[string]string{
		"htmx.min.js": "https://unpkg.com/htmx.org/dist/htmx.min.js",
		// "tailwind.js": "https://cdn.tailwindcss.com",
		"jsonenc.js": "https://unpkg.com/htmx.org/dist/ext/json-enc.js",
		// "alpine.js":  "https://unpkg.com/alpinejs/dist/cdn.min.js",
		"alpine.js":      "https://unpkg.com/alpinejs@2.8.2/dist/alpine.js",
		"hyperscript.js": "https://unpkg.com/hyperscript.org",
		"cally.js":       "https://unpkg.com/cally",
		// "tinymce.js":     "https://cdn.tiny.cloud/1/no-api-key/tinymce/5/tinymce.min.js",
		"tinymce.js": "https://cdn.jsdelivr.net/npm/@tinymce/tinymce-webcomponent@2/dist/tinymce-webcomponent.min.js",
	}
)

func InitAssets() {

	log.Println("Initializing assets")
	baseAssetDir := "./assets/dist"
	// create asset directory if it doesn't exist
	if _, err := os.Stat(baseAssetDir); os.IsNotExist(err) {
		log.Println("Creating asset directory")
		err := os.MkdirAll(baseAssetDir, 0755)
		if err != nil {
			log.Println("Error creating asset directory")
			log.Println(err)
		}
	}

	for assetName, assetURL := range Assets {
		// check if asset exists
		if _, err := os.Stat(baseAssetDir + "/" + assetName); os.IsNotExist(err) {
			log.Println("Downloading asset", assetName)
			// download asset
			log.Println(baseAssetDir+"/"+assetName, assetURL)
			err := DownloadFile(baseAssetDir+"/"+assetName, assetURL)
			if err != nil {
				log.Println("Error downloading asset", assetName)
				log.Println(err)
			}
		}
		// DownloadFile(baseAssetDir+"/"+assetName, assetURL)
	}

	// Copy 'assets/src/style.css' to 'assets/dist/style.css'

	// if _, err := os.Stat(baseAssetDir + "/style.css"); os.IsNotExist(err) {
	// 	log.Println("Copying style.css")
	// 	err := CopyFile("./assets/src/style.css", baseAssetDir+"/style.css")
	// 	if err != nil {
	// 		log.Println("Error copying style.css")
	// 		log.Println(err)
	// 	}
	// }

}

func DownloadFile(filepath string, url string) error {

	CheckRedirect := func(req *http.Request, via []*http.Request) error {
		log.Println("Redirected to", req.URL)
		return nil
	}

	// Create a custom HTTP client with the CheckRedirect function
	client := &http.Client{
		CheckRedirect: CheckRedirect,
	}

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the file
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Write the file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func CreateShortLink(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	if length <= 0 {
		return "", errors.New("length must be greater than zero")
	}

	result := make([]byte, length)
	for i := 0; i < length; i++ {
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[randomIndex.Int64()]
	}

	return string(result), nil
}

func CopyFile(src string, dst string) error {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return errors.New(src + " is not a regular file")
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()
	_, err = io.Copy(destination, source)
	if err == nil {
		err = os.Chmod(dst, sourceFileStat.Mode())
	}

	return err
}

// Check if a ip is private.
func IsPrivateIP(ip string) bool {
	if len(ip) == 0 {
		return false
	}
	ipAddress := net.ParseIP(ip)
	return ipAddress.IsPrivate()
}

func GetUserFromJWT(c *fiber.Ctx) (userID uint, err error) {
	jwtToken := c.Cookies("jwt")

	hmacSampleSecret := []byte(os.Getenv("JWT_SECRET"))

	// log.Println(hmacSampleSecret)

	token, err := jwt.Parse(jwtToken, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			// return nil, fmt.Errorf("UNEXPECTED SIGNING METHOD: %v", token.Header["alg"])
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// hmacSampleSecret is a []byte containing your secret, e.g. []byte("my_secret_key")
		return hmacSampleSecret, nil
	})
	if err != nil {
		return 0, err
	}
	claims := token.Claims.(jwt.MapClaims)
	user_id := claims["user_id"]
	if user_id == nil {
		return 0, errors.New("user_id not found in claims")
	}

	f := user_id.(float64)

	// Check if the float64 value is non-negative and within the range of uint
	if f < 0 {
		// fmt.Println("Cannot convert negative float to uint")
		return 0, errors.New("cannot convert negative float to uint")

	}
	if f != math.Trunc(f) {
		// fmt.Println("Cannot convert non-integral float to uint")
		return 0, errors.New("cannot convert non-integral float to uint")
	}
	if f > float64(math.MaxUint) {
		// fmt.Println("Float value exceeds the maximum value of uint")
		return 0, errors.New("float value exceeds the maximum value of uint")
	}

	userID = uint(f)

	return userID, nil

}
