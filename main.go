package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

const (
	classifyEndpoint = "http://localhost:8080/classify"
	checkEndpoint    = "http://localhost:8081/check"
)

type CheckRequest struct {
	Image       []byte `json:"image"`
	NamaPetani  string `json:"nama_petani"`
	Alamat      string `json:"alamat"`
	Kota        string `json:"kota"`
	ContentType string `json:"content_type"`
}

type CheckResponse struct {
	Duplicate bool   `json:"duplicate"`
	Message   string `json:"message"`
}

func main() {
	r := gin.Default()
	r.POST("/upload", handleUpload)
	if err := r.Run(":8090"); err != nil {
		panic(err)
	}
}

func handleUpload(c *gin.Context) {
	// Read the form data from the request
	namaPetani := c.PostForm("nama_petani")
	alamat := c.PostForm("alamat")
	kota := c.PostForm("kota")
	imageHeader, err := c.FormFile("image")
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Missing image in form data"})
		return
	}

	image, err := imageHeader.Open()
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Failed to open uploaded file"})
	}

	defer image.Close()

	// Classify the image
	isKolam, err := classifyImage(image)

	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to classify image"})
		return
	}
	if !isKolam {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Image is not a kolam"})
		return
	}

	// Check if the image is a duplicate
	isDuplicate, err := checkDuplicate(image, namaPetani, alamat, kota, imageHeader.Filename)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to check duplicate"})
		return
	}
	if isDuplicate {
		c.JSON(http.StatusOK, gin.H{"message": "Image is a duplicate"})
		return
	}

	// Success
	c.JSON(http.StatusOK, gin.H{"message": "Image classified as kolam"})
}

func classifyImage(imageFile multipart.File) (bool, error) {
	// Create a new multipart buffer to store the image file
	buffer := new(bytes.Buffer)
	writer := multipart.NewWriter(buffer)
	part, err := writer.CreateFormFile("image", "image.jpg")
	if err != nil {
		return false, err
	}
	_, err = io.Copy(part, imageFile)
	if err != nil {
		return false, err
	}
	writer.Close()

	// Send the image to the classification endpoint
	resp, err := http.Post(classifyEndpoint, writer.FormDataContentType(), buffer)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("classification endpoint returned %d status code", resp.StatusCode)
	}

	var response struct {
		Result string `json:"result"`
	}

	err = json.NewDecoder(resp.Body).Decode(&response)

	if err != nil {
		return false, err
	}

	result := (response.Result == "kolam")
	return result, nil
}

func checkDuplicate(imageFile multipart.File, namaPetani string, alamat string, kota string, nameFile string) (bool, error) {
	// Check file extension
	ext := filepath.Ext(nameFile)
	if ext != ".png" && ext != ".jpg" {
		return false, errors.New("unsupported file type")
	}

	// Create a new multipart buffer to store the image file and metadata
	buffer := new(bytes.Buffer)
	writer := multipart.NewWriter(buffer)

	part, err := writer.CreateFormFile("image", ext)
	if err != nil {
		return false, err
	}

	// Copy the image file to the form part
	_, err = io.Copy(part, imageFile)
	if err != nil {
		return false, err
	}

	// Write other form fields
	err = writer.WriteField("nama_petani", namaPetani)
	if err != nil {
		return false, err
	}
	err = writer.WriteField("alamat", alamat)
	if err != nil {
		return false, err
	}
	err = writer.WriteField("kota", kota)
	if err != nil {
		return false, err
	}

	err = writer.Close()
	if err != nil {
		return false, err
	}

	// Send the image and metadata to the duplicate check endpoint
	resp, err := http.Post(checkEndpoint, writer.FormDataContentType(), buffer)
	if err != nil {
		return false, err
	}

	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("duplicate check endpoint returned %d status code", resp.StatusCode)
	}

	var response CheckResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return false, err
	}

	return response.Duplicate, nil
}
