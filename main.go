package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

// XML structures for parsing
type XMEML struct {
	XMLName  xml.Name `xml:"xmeml"`
	Sequence Sequence `xml:"sequence"`
}

type Sequence struct {
	ID    string `xml:"id,attr"`
	Name  string `xml:"name"`
	Media Media  `xml:"media"`
}

type Media struct {
	Video Video `xml:"video"`
}

type Video struct {
	Tracks []Track `xml:"track"`
}

type Track struct {
	Name      string     `xml:"name"`
	ClipItems []ClipItem `xml:"clipitem"`
}

type ClipItem struct {
	Name  string `xml:"name"`
	In    string `xml:"in"`
	Out   string `xml:"out"`
	Start string `xml:"start"`
	End   string `xml:"end"`
	File  File   `xml:"file"`
}

type File struct {
	ID       string   `xml:"id,attr"`
	Name     string   `xml:"name"`
	PathURL  string   `xml:"pathurl"`
	Timecode Timecode `xml:"timecode"`
}

type Timecode struct {
	String string `xml:"string"`
}

// formatTimecode converts frame number to timecode format HH:MM:SS:FF
func formatTimecode(framesStr string, fps int) string {
	if framesStr == "" {
		return "00:00:00:00"
	}

	frames, err := strconv.Atoi(framesStr)
	if err != nil {
		return "00:00:00:00"
	}

	hours := frames / (3600 * fps)
	frames %= 3600 * fps
	minutes := frames / (60 * fps)
	frames %= 60 * fps
	seconds := frames / fps
	frames %= fps

	return fmt.Sprintf("%02d:%02d:%02d:%02d", hours, minutes, seconds, frames)
}

// parseTimecodeToFrames converts a timecode to frame count
func parseTimecodeToFrames(timecode string, fps int) int {
	parts := strings.Split(timecode, ":")
	if len(parts) != 4 {
		return 0
	}

	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])
	seconds, _ := strconv.Atoi(parts[2])
	frames, _ := strconv.Atoi(parts[3])

	return hours*3600*fps + minutes*60*fps + seconds*fps + frames
}

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("XML to EDL Converter")
	myWindow.Resize(fyne.NewSize(400, 200))

	var xmlFilePath fyne.URI
	var edlContent []string

	openButton := widget.NewButton("Open XML File", func() {
		fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, myWindow)
				return
			}
			if reader == nil {
				// User cancelled
				return
			}
			defer reader.Close()

			xmlFilePath = reader.URI()

			xmlData, readErr := os.ReadFile(xmlFilePath.Path())
			if readErr != nil {
				dialog.ShowError(fmt.Errorf("error reading XML file: %w", readErr), myWindow)
				return
			}

			var xmeml XMEML
			parseErr := xml.Unmarshal(xmlData, &xmeml)
			if parseErr != nil {
				dialog.ShowError(fmt.Errorf("error parsing XML: %w", parseErr), myWindow)
				return
			}

			sequenceName := xmeml.Sequence.Name
			if sequenceName == "" {
				sequenceName = "Sequence 1"
			}

			currentEdlContent := []string{
				fmt.Sprintf("TITLE: %s", sequenceName),
				"FCM: NON-DROP FRAME",
				"", // Empty line after header
			}

			eventNumber := 1
			fps := 30 // Assuming 30 fps

			for _, track := range xmeml.Sequence.Media.Video.Tracks {
				for _, clip := range track.ClipItems {
					fileID := clip.File.ID
					if fileID == "" {
						baseFilename := filepath.Base(clip.File.PathURL)
						ext := filepath.Ext(baseFilename)
						fileID = strings.TrimSuffix(baseFilename, ext)
						if len(fileID) > 8 {
							fileID = fileID[:8]
						}
					}
					sourceTc := clip.File.Timecode.String
					filename := filepath.Base(clip.File.PathURL)
					if filename == "" {
						filename = fmt.Sprintf("%s.mov", clip.Name)
					}

					baseFrames := parseTimecodeToFrames(sourceTc, fps)
					inFrames, _ := strconv.Atoi(clip.In)
					outFrames, _ := strconv.Atoi(clip.Out)

					sourceInTc := formatTimecode(strconv.Itoa(inFrames+baseFrames), fps)
					sourceOutTc := formatTimecode(strconv.Itoa(outFrames+baseFrames), fps)
					recordInTc := formatTimecode(clip.Start, fps)
					recordOutTc := formatTimecode(clip.End, fps)

					edlEntry := []string{
						fmt.Sprintf("%03d    %s V C        %s %s %s %s",
							eventNumber, fileID, sourceInTc, sourceOutTc, recordInTc, recordOutTc),
						fmt.Sprintf("* FROM CLIP NAME: %s", filename),
						"",
					}
					currentEdlContent = append(currentEdlContent, edlEntry...)
					eventNumber++
				}
			}
			edlContent = currentEdlContent // Store for saving

			// Suggest a filename for saving
			originalPath := xmlFilePath.Path()
			// dir := filepath.Dir(originalPath) // This line can be removed
			baseName := filepath.Base(originalPath)
			ext := filepath.Ext(baseName)
			suggestedEdlName := strings.TrimSuffix(baseName, ext) + "_converted.edl"

			saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
				if err != nil {
					dialog.ShowError(err, myWindow)
					return
				}
				if writer == nil {
					// User cancelled
					return
				}
				defer writer.Close()

				edlOutput := strings.Join(edlContent, "\n")
				_, writeErr := writer.Write([]byte(edlOutput))
				if writeErr != nil {
					dialog.ShowError(fmt.Errorf("error writing EDL file: %w", writeErr), myWindow)
					return
				}
				numClips := (len(edlContent) - 3) / 3
				dialog.ShowInformation("Success", fmt.Sprintf("Successfully converted %d clips to EDL format.\nOutput saved to: %s", numClips, writer.URI().Path()), myWindow)

			}, myWindow)

			// Set suggested filename and directory
			saveDialog.SetFileName(suggestedEdlName)
			parentURI, _ := storage.Parent(xmlFilePath)
			if parentURI != nil {
				listableParentURI, _ := storage.ListerForURI(parentURI)
				saveDialog.SetLocation(listableParentURI)
			}

			saveDialog.Show()

		}, myWindow)
		fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".xml"}))
		fileDialog.Show()
	})

	myWindow.SetContent(container.NewCenter(openButton))
	myWindow.ShowAndRun()
}
