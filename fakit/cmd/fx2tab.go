// Copyright © 2016 Wei Shen <shenwei356@gmail.com>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cmd

import (
	"fmt"
	"runtime"

	"github.com/brentp/xopen"
	"github.com/shenwei356/bio/seq"
	"github.com/shenwei356/bio/seqio/fastx"
	"github.com/spf13/cobra"
)

// fx2tabCmd represents the fx2tab command
var fx2tabCmd = &cobra.Command{
	Use:   "fx2tab",
	Short: "covert FASTA/Q to tabular format (with length/GC content/GC skew)",
	Long: `covert FASTA/Q to tabular format, and provide various information,
like sequence length, GC content/GC skew.

`,
	Run: func(cmd *cobra.Command, args []string) {
		config := getConfigs(cmd)
		alphabet := config.Alphabet
		idRegexp := config.IDRegexp
		chunkSize := config.ChunkSize
		bufferSize := config.BufferSize
		outFile := config.OutFile
		seq.AlphabetGuessSeqLenghtThreshold = config.AlphabetGuessSeqLength
		seq.ValidateSeq = false
		runtime.GOMAXPROCS(config.Threads)

		files := getFileList(args)

		onlyID := getFlagBool(cmd, "only-id")
		printLength := getFlagBool(cmd, "length")
		printGC := getFlagBool(cmd, "gc")
		printGCSkew := getFlagBool(cmd, "gc-skew")
		baseContents := getFlagStringSlice(cmd, "base-content")
		onlyName := getFlagBool(cmd, "name")
		printTitle := getFlagBool(cmd, "header-line")

		outfh, err := xopen.Wopen(outFile)
		checkError(err)
		defer outfh.Close()

		if printTitle {
			outfh.WriteString("#name\tseq\tqual")
			if printLength {
				outfh.WriteString("\tlength")
			}
			if printGC {
				outfh.WriteString("\tGC")
			}
			if printGCSkew {
				outfh.WriteString("\tGC-Skew")
			}
			if len(baseContents) > 0 {
				for _, bc := range baseContents {
					outfh.WriteString(fmt.Sprintf("\t%s", bc))
				}
			}
			outfh.WriteString("\n")
		}

		var name []byte
		var g, c float64
		for _, file := range files {
			fastxReader, err := fastx.NewReader(alphabet, file, bufferSize, chunkSize, idRegexp)
			checkError(err)
			for chunk := range fastxReader.Ch {
				checkError(chunk.Err)

				for _, record := range chunk.Data {
					if onlyID {
						name = record.ID
					} else {
						name = record.Name
					}
					if onlyName {
						outfh.WriteString(fmt.Sprintf("%s\t%s\t%s", name, "", ""))
					} else {
						//outfh.WriteString(fmt.Sprintf("%s\t%s\t%s", name,
						//	record.Seq.Seq, record.Seq.Qual))
						outfh.WriteString(fmt.Sprintf("%s\t", name))
						outfh.Write(record.Seq.Seq)
						outfh.WriteString("\t")
						outfh.Write(record.Seq.Qual)

					}

					if printLength {
						outfh.WriteString(fmt.Sprintf("\t%d", len(record.Seq.Seq)))
					}
					if printGC || printGCSkew {
						g = record.Seq.BaseContent("G")
						c = record.Seq.BaseContent("C")
					}

					if printGC {
						outfh.WriteString(fmt.Sprintf("\t%.2f", (g+c)*100))
					}
					if printGCSkew {
						outfh.WriteString(fmt.Sprintf("\t%.2f", (g-c)/(g+c)*100))
					}

					if len(baseContents) > 0 {
						for _, bc := range baseContents {
							outfh.WriteString(fmt.Sprintf("\t%.2f", record.Seq.BaseContent(bc)*100))
						}
					}
					outfh.WriteString("\n")
				}
			}
		}
	},
}

func init() {
	RootCmd.AddCommand(fx2tabCmd)

	fx2tabCmd.Flags().BoolP("length", "l", false, "print sequence length")
	fx2tabCmd.Flags().BoolP("gc", "g", false, "print GC content")
	fx2tabCmd.Flags().BoolP("gc-skew", "G", false, "print GC-Skew")
	fx2tabCmd.Flags().StringSliceP("base-content", "B", []string{}, "print base content. (case ignored, multiple values supported) e.g. -B AT -B N")
	fx2tabCmd.Flags().BoolP("only-id", "i", false, "print ID instead of full head")
	fx2tabCmd.Flags().BoolP("name", "n", false, "only print names (no sequences and qualities)")
	fx2tabCmd.Flags().BoolP("header-line", "H", false, "print header line")
}
