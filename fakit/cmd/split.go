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
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/brentp/xopen"
	"github.com/shenwei356/bio/seq"
	"github.com/shenwei356/bio/seqio/fai"
	"github.com/shenwei356/bio/seqio/fastx"
	"github.com/spf13/cobra"
)

// splitCmd represents the split command
var splitCmd = &cobra.Command{
	Use:   "split",
	Short: "split sequences into files by id/seq region/size/parts",
	Long: fmt.Sprintf(`split sequences into files by name ID, subsequence of given region,
part size or number of parts.

The definition of region is 1-based and with some custom design.

Examples:
%s
`, regionExample),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 1 {
			checkError(fmt.Errorf("no more than one file needed (%d)", len(args)))
		}

		config := getConfigs(cmd)
		alphabet := config.Alphabet
		idRegexp := config.IDRegexp
		chunkSize := config.ChunkSize
		bufferSize := config.BufferSize
		lineWidth := config.LineWidth
		outFile := config.OutFile
		quiet := config.Quiet
		seq.AlphabetGuessSeqLenghtThreshold = config.AlphabetGuessSeqLength
		seq.ValidateSeq = false
		fai.MapWholeFile = false
		runtime.GOMAXPROCS(config.Threads)

		files := getFileList(args)

		size := getFlagInt(cmd, "by-size")
		if size < 0 {
			checkError(fmt.Errorf("value of flag -s (--size) should be greater than 0: %d ", size))
		}
		part := getFlagInt(cmd, "by-part")
		if part < 0 {
			checkError(fmt.Errorf("value of flag -p (--part) should be greater than 0: %d ", part))
		}

		byID := getFlagBool(cmd, "by-id")
		region := getFlagString(cmd, "by-region")
		twoPass := getFlagBool(cmd, "two-pass")
		keepTemp := getFlagBool(cmd, "keep-temp")
		if keepTemp && !twoPass {
			checkError(fmt.Errorf("flag -k (--keep-temp) must be used with flag -2 (--two-pass)"))
		}
		usingMD5 := getFlagBool(cmd, "md5")
		if usingMD5 && region == "" {
			checkError(fmt.Errorf("flag -m (--md5) must be used with flag -r (--region)"))
		}
		dryRun := getFlagBool(cmd, "dry-run")

		outfh, err := xopen.Wopen(outFile)
		checkError(err)
		defer outfh.Close()

		if len(files) > 1 {
			checkError(fmt.Errorf("no more than one file should be given"))
		}

		file := files[0]
		var fileName, fileExt string
		if isStdin(file) {
			fileName, fileExt = "stdin", ".fastx"
		} else {
			fileName, fileExt = filepathTrimExtension(file)
		}
		renameFileExt := true
		var outfile string

		if size > 0 {
			if !twoPass {
				if !quiet {
					log.Infof("split into %d seqs per file", size)
				}

				i := 1
				records := []*fastx.Record{}

				fastxReader, err := fastx.NewReader(alphabet, file, bufferSize, chunkSize, idRegexp)
				checkError(err)

				for chunk := range fastxReader.Ch {
					checkError(chunk.Err)
					for _, record := range chunk.Data {
						if renameFileExt {
							if len(record.Seq.Qual) > 0 {
								fileExt = ".fastq"
							} else {
								fileExt = ".fasta"
							}
							renameFileExt = false
						}
						records = append(records, record)
						if len(records) == size {
							outfile = fmt.Sprintf("%s.part_%03d%s", fileName, i, fileExt)
							writeSeqs(records, outfile, lineWidth, quiet, dryRun)
							i++
							records = []*fastx.Record{}
						}
					}
				}
				if len(records) > 0 {
					outfile = fmt.Sprintf("%s.part_%03d%s", fileName, i, fileExt)
					writeSeqs(records, outfile, lineWidth, quiet, dryRun)
				}

				return
			}

			var alphabet2 *seq.Alphabet

			newFile := file

			if isStdin(file) || !isPlainFile(file) {
				if isStdin(file) {
					newFile = "stdin" + ".fastx"
				} else {
					newFile = file + ".fastx"
				}
				if !quiet {
					log.Infof("read and write sequences to tempory file: %s ...", newFile)
				}

				copySeqs(file, newFile)

				var isFastq bool
				var err error
				alphabet2, isFastq, err = fastx.GuessAlphabet(newFile)
				checkError(err)
				if renameFileExt {
					if isFastq {
						fileExt = ".fastq"
					} else {
						fileExt = ".fasta"
					}
					renameFileExt = false
				}
				if isFastq {
					checkError(os.Remove(newFile))
					checkError(fmt.Errorf("Sorry, two-pass mode does not support FASTQ format"))
				}
			}
			fileExt = ".fasta"

			if !quiet {
				log.Infof("create and read FASTA index ...")
			}
			faidx := getFaidx(newFile, `^(.+)$`)
			defer func() {
				checkError(faidx.Close())
			}()

			if !quiet {
				log.Infof("read sequence IDs from FASTA index ...")
			}
			IDs, _, err := getSeqIDAndLengthFromFaidxFile(newFile + ".fakit.fai")
			checkError(err)
			if !quiet {
				log.Infof("%d sequences loaded", len(IDs))
			}

			n := 1
			if len(IDs) > 0 {
				outfile = fmt.Sprintf("%s.part_%03d%s", fileName, n, fileExt)
				if !dryRun {
					outfh, err = xopen.Wopen(outfile)
					checkError(err)
				}
			}
			j := 0
			var record *fastx.Record
			for _, chr := range IDs {
				if !dryRun {
					r, ok := faidx.Index[chr]
					if !ok {
						checkError(fmt.Errorf(`sequence (%s) not found in file: %s`, chr, newFile))
					}

					sequence := subseqByFaix(faidx, chr, r, 1, -1)
					record, err = fastx.NewRecord(alphabet2, []byte(chr), []byte(chr), sequence)
					checkError(err)

					record.FormatToWriter(outfh, lineWidth)
				}
				j++
				if j == size {
					if !quiet {
						log.Infof("write %d sequences to file: %s\n", j, outfile)
					}
					n++
					outfile = fmt.Sprintf("%s.part_%03d%s", fileName, n, fileExt)
					if !dryRun {
						outfh.Close()
						outfh, err = xopen.Wopen(outfile)
						checkError(err)
					}
					j = 0
				}
			}
			if j > 0 && !quiet {
				log.Infof("write %d sequences to file: %s\n", j, outfile)
			}
			if !dryRun {
				outfh.Close()
			}

			if (isStdin(file) || !isPlainFile(file)) && !keepTemp {
				checkError(os.Remove(newFile))
				checkError(os.Remove(newFile + ".fakit.fai"))
			}
			return
		}

		if part > 0 {
			if !quiet {
				log.Infof("split into %d parts", part)
			}
			if !twoPass {
				i := 1
				records := []*fastx.Record{}

				if !quiet {
					log.Info("read sequences ...")
				}
				allRecords, err := fastx.GetSeqs(file, alphabet, bufferSize, chunkSize, idRegexp)
				checkError(err)
				if !quiet {
					log.Infof("read %d sequences", len(allRecords))
				}

				n := len(allRecords)
				if n%part > 0 {
					size = int(n/part) + 1
					if n%size == 0 {
						if !quiet {
							log.Infof("corrected: split into %d parts", n/size)
						}
					}
				} else {
					size = int(n / part)
				}

				for _, record := range allRecords {
					if renameFileExt {
						if len(record.Seq.Qual) > 0 {
							fileExt = ".fastq"
						} else {
							fileExt = ".fasta"
						}
						renameFileExt = false
					}
					records = append(records, record)
					if len(records) == size {
						outfile = fmt.Sprintf("%s.part_%03d%s", fileName, i, fileExt)
						writeSeqs(records, outfile, lineWidth, quiet, dryRun)
						i++
						records = []*fastx.Record{}
					}
				}
				if len(records) > 0 {
					outfile = fmt.Sprintf("%s.part_%03d%s", fileName, i, fileExt)
					writeSeqs(records, outfile, lineWidth, quiet, dryRun)
				}
				return
			}

			var alphabet2 *seq.Alphabet

			newFile := file

			if isStdin(file) || !isPlainFile(file) {
				if isStdin(file) {
					newFile = "stdin" + ".fastx"
				} else {
					newFile = file + ".fastx"
				}
				if !quiet {
					log.Infof("read and write sequences to tempory file: %s ...", newFile)
				}

				copySeqs(file, newFile)

				var isFastq bool
				var err error
				alphabet2, isFastq, err = fastx.GuessAlphabet(newFile)
				checkError(err)
				if renameFileExt {
					if isFastq {
						fileExt = ".fastq"
					} else {
						fileExt = ".fasta"
					}
					renameFileExt = false
				}
				if isFastq {
					checkError(os.Remove(newFile))
					checkError(fmt.Errorf("Sorry, two-pass mode does not support FASTQ format"))
				}
			}
			fileExt = ".fasta"

			if !quiet {
				log.Infof("create and read FASTA index ...")
			}
			faidx := getFaidx(newFile, `^(.+)$`)
			defer func() {
				checkError(faidx.Close())
			}()

			if !quiet {
				log.Infof("read sequence IDs from FASTA index ...")
			}
			IDs, _, err := getSeqIDAndLengthFromFaidxFile(newFile + ".fakit.fai")
			checkError(err)
			if !quiet {
				log.Infof("%d sequences loaded", len(IDs))
			}

			N := len(IDs)
			if N%part > 0 {
				size = int(N/part) + 1
				if N%size == 0 {
					if !quiet {
						log.Infof("corrected: split into %d parts", N/size)
					}
				}
			} else {
				size = int(N / part)
			}

			n := 1
			if len(IDs) > 0 {
				outfile = fmt.Sprintf("%s.part_%03d%s", fileName, n, fileExt)
				if !dryRun {
					outfh, err = xopen.Wopen(outfile)
					checkError(err)
				}
			}
			j := 0
			var record *fastx.Record
			for _, chr := range IDs {
				if !dryRun {
					r, ok := faidx.Index[chr]
					if !ok {
						checkError(fmt.Errorf(`sequence (%s) not found in file: %s`, chr, newFile))
					}

					sequence := subseqByFaix(faidx, chr, r, 1, -1)
					record, err = fastx.NewRecord(alphabet2, []byte(chr), []byte(chr), sequence)
					checkError(err)

					record.FormatToWriter(outfh, lineWidth)
				}
				j++
				if j == size {
					if !quiet {
						log.Infof("write %d sequences to file: %s\n", j, outfile)
					}
					n++
					outfile = fmt.Sprintf("%s.part_%03d%s", fileName, n, fileExt)
					if !dryRun {
						outfh.Close()
						outfh, err = xopen.Wopen(outfile)
						checkError(err)
					}
					j = 0
				}
			}
			if j > 0 && !quiet {
				log.Infof("write %d sequences to file: %s\n", j, outfile)
			}
			if !dryRun {
				outfh.Close()
			}

			if (isStdin(file) || !isPlainFile(file)) && !keepTemp {
				checkError(os.Remove(newFile))
				checkError(os.Remove(newFile + ".fakit.fai"))
			}
			return
		}

		if byID {
			if !quiet {
				log.Infof("split by ID. idRegexp: %s", idRegexp)
			}

			if !twoPass {
				if !quiet {
					log.Info("read sequences ...")
				}
				allRecords, err := fastx.GetSeqs(file, alphabet, bufferSize, chunkSize, idRegexp)
				checkError(err)
				if !quiet {
					log.Infof("read %d sequences", len(allRecords))
				}

				recordsByID := make(map[string][]*fastx.Record)

				var id string
				for _, record := range allRecords {
					if renameFileExt {
						if len(record.Seq.Qual) > 0 {
							fileExt = ".fastq"
						} else {
							fileExt = ".fasta"
						}
						renameFileExt = false
					}
					id = string(record.ID)
					if _, ok := recordsByID[id]; !ok {
						recordsByID[id] = []*fastx.Record{}
					}
					recordsByID[id] = append(recordsByID[id], record)
				}

				var outfile string
				for id, records := range recordsByID {
					outfile = fmt.Sprintf("%s.id_%s%s", fileName, id, fileExt)
					writeSeqs(records, outfile, lineWidth, quiet, dryRun)
				}
				return
			}

			var alphabet2 *seq.Alphabet

			newFile := file

			if isStdin(file) || !isPlainFile(file) {
				if isStdin(file) {
					newFile = "stdin" + ".fastx"
				} else {
					newFile = file + ".fastx"
				}
				if !quiet {
					log.Infof("read and write sequences to tempory file: %s ...", newFile)
				}

				copySeqs(file, newFile)

				var isFastq bool
				var err error
				alphabet2, isFastq, err = fastx.GuessAlphabet(newFile)
				checkError(err)
				if renameFileExt {
					if isFastq {
						fileExt = ".fastq"
					} else {
						fileExt = ".fasta"
					}
					renameFileExt = false
				}
				if isFastq {
					checkError(os.Remove(newFile))
					checkError(fmt.Errorf("Sorry, two-pass mode does not support FASTQ format"))
				}
			}
			fileExt = ".fasta"

			if !quiet {
				log.Infof("create and read FASTA index ...")
			}
			faidx := getFaidx(newFile, `^(.+)$`)
			defer func() {
				checkError(faidx.Close())
			}()

			if !quiet {
				log.Infof("read sequence IDs from FASTA index ...")
			}
			IDs, _, err := getSeqIDAndLengthFromFaidxFile(newFile + ".fakit.fai")
			checkError(err)
			if !quiet {
				log.Infof("%d sequences loaded", len(IDs))
			}

			idRe, err := regexp.Compile(idRegexp)
			if err != nil {
				checkError(fmt.Errorf("fail to compile regexp: %s", idRegexp))
			}

			idsMap := make(map[string][]string)
			id2name := make(map[string]string)
			for _, ID := range IDs {
				id := string(fastx.ParseHeadID(idRe, []byte(ID)))
				if _, ok := idsMap[id]; !ok {
					idsMap[id] = []string{}
				}
				idsMap[id] = append(idsMap[id], id)
				id2name[id] = ID
			}

			var outfile string
			var record *fastx.Record
			for id, ids := range idsMap {

				outfile = fmt.Sprintf("%s.id_%s%s", fileName, id, fileExt)
				if !dryRun {
					outfh, err = xopen.Wopen(outfile)
					checkError(err)
				}

				for _, chr := range ids {
					if !dryRun {
						chr = id2name[chr]
						r, ok := faidx.Index[chr]
						if !ok {
							checkError(fmt.Errorf(`sequence (%s) not found in file: %s`, chr, newFile))
						}

						sequence := subseqByFaix(faidx, chr, r, 1, -1)
						record, err = fastx.NewRecord(alphabet2, []byte(chr), []byte(chr), sequence)
						checkError(err)

						record.FormatToWriter(outfh, lineWidth)
					}
				}

				if !quiet {
					log.Infof("write %d sequences to file: %s\n", len(ids), outfile)
				}
				if !dryRun {
					outfh.Close()
				}
			}

			if (isStdin(file) || !isPlainFile(file)) && !keepTemp {
				checkError(os.Remove(newFile))
				checkError(os.Remove(newFile + ".fakit.fai"))
			}
			return
		}

		if region != "" {
			if !reRegion.MatchString(region) {
				checkError(fmt.Errorf(`invalid region: %s. type "fakit subseq -h" for more examples`, region))
			}
			r := strings.Split(region, ":")
			start, err := strconv.Atoi(r[0])
			checkError(err)
			end, err := strconv.Atoi(r[1])
			checkError(err)
			if start == 0 || end == 0 {
				checkError(fmt.Errorf("both start and end should not be 0"))
			}
			if start < 0 && end > 0 {
				checkError(fmt.Errorf("when start < 0, end should not > 0"))
			}

			if !quiet {
				log.Infof("split by region: %s", region)
			}

			if !twoPass {
				if !quiet {
					log.Info("read sequences ...")
				}
				allRecords, err := fastx.GetSeqs(file, alphabet, bufferSize, chunkSize, idRegexp)
				checkError(err)
				if !quiet {
					log.Infof("read %d sequences", len(allRecords))
				}

				recordsBySeqs := make(map[string][]*fastx.Record)

				var subseq string
				var s, e int
				var ok bool
				for _, record := range allRecords {
					if renameFileExt {
						if len(record.Seq.Qual) > 0 {
							fileExt = ".fastq"
						} else {
							fileExt = ".fasta"
						}
						renameFileExt = false
					}
					s, e, ok = seq.SubLocation(len(record.Seq.Seq), start, end)
					if !ok {
						checkError(fmt.Errorf("region (%s) not match sequence (%s) with length of %d", region, record.Name, len(record.Seq.Seq)))
					}
					if usingMD5 {
						subseq = string(MD5(record.Seq.SubSeq(s, e).Seq))
					} else {
						subseq = string(record.Seq.SubSeq(s, e).Seq)
					}
					if _, ok := recordsBySeqs[subseq]; !ok {
						recordsBySeqs[subseq] = []*fastx.Record{}
					}
					recordsBySeqs[subseq] = append(recordsBySeqs[subseq], record)
				}

				var outfile string
				for subseq, records := range recordsBySeqs {
					outfile = fmt.Sprintf("%s.region_%d:%d_%s%s", fileName, start, end, subseq, fileExt)
					writeSeqs(records, outfile, lineWidth, quiet, dryRun)
				}
				return
			}

			var alphabet2 *seq.Alphabet

			newFile := file

			if isStdin(file) || !isPlainFile(file) {
				if isStdin(file) {
					newFile = "stdin" + ".fastx"
				} else {
					newFile = file + ".fastx"
				}
				if !quiet {
					log.Infof("read and write sequences to tempory file: %s ...", newFile)
				}

				copySeqs(file, newFile)

				var isFastq bool
				var err error
				alphabet2, isFastq, err = fastx.GuessAlphabet(newFile)
				if renameFileExt {
					if isFastq {
						fileExt = ".fastq"
					} else {
						fileExt = ".fasta"
					}
					renameFileExt = false
				}
				checkError(err)
				if isFastq {
					checkError(os.Remove(newFile))
					checkError(fmt.Errorf("Sorry, two-pass mode does not support FASTQ format"))
				}
			}
			fileExt = ".fasta"

			if !quiet {
				log.Infof("read sequence IDs and sequence region from FASTA file ...")
			}
			region2name := make(map[string][]string)

			fastxReader, err := fastx.NewReader(alphabet2, newFile, bufferSize, chunkSize, idRegexp)
			checkError(err)
			var name string
			var subseq string
			var s, e int
			var ok bool
			for chunk := range fastxReader.Ch {
				checkError(chunk.Err)
				for _, record := range chunk.Data {
					s, e, ok = seq.SubLocation(len(record.Seq.Seq), start, end)
					if !ok {
						checkError(fmt.Errorf("region (%s) not match sequence (%s) with length of %d", region, record.Name, len(record.Seq.Seq)))
					}
					if usingMD5 {
						subseq = string(MD5(record.Seq.SubSeq(s, e).Seq))
					} else {
						subseq = string(record.Seq.SubSeq(s, e).Seq)
					}
					name = string(record.Name)
					if _, ok := region2name[subseq]; !ok {
						region2name[subseq] = []string{}
					}
					region2name[subseq] = append(region2name[subseq], name)
				}
			}

			if !quiet {
				log.Infof("create and read FASTA index ...")
			}
			faidx := getFaidx(newFile, `^(.+)$`)
			defer func() {
				checkError(faidx.Close())
			}()

			var outfile string
			var record *fastx.Record
			for subseq, chrs := range region2name {
				outfile = fmt.Sprintf("%s.region_%d:%d_%s%s", fileName, start, end, subseq, fileExt)
				if !dryRun {
					outfh, err = xopen.Wopen(outfile)
					checkError(err)
				}

				for _, chr := range chrs {
					if !dryRun {
						r, ok := faidx.Index[chr]
						if !ok {
							checkError(fmt.Errorf(`sequence (%s) not found in file: %s`, chr, newFile))
						}

						sequence := subseqByFaix(faidx, chr, r, 1, -1)
						record, err = fastx.NewRecord(alphabet2, []byte(chr), []byte(chr), sequence)
						checkError(err)

						record.FormatToWriter(outfh, lineWidth)
					}
				}

				if !quiet {
					log.Infof("write %d sequences to file: %s\n", len(chrs), outfile)
				}
				if !dryRun {
					outfh.Close()
				}
			}
			return
		}

		checkError(fmt.Errorf(`one of flags should be given: -s/-p/-i/-r. type "fakit split -h" for help`))
	},
}

func init() {
	RootCmd.AddCommand(splitCmd)

	splitCmd.Flags().IntP("by-size", "s", 0, "split squences into multi parts with N sequences")
	splitCmd.Flags().IntP("by-part", "p", 0, "split squences into N parts")
	splitCmd.Flags().BoolP("by-id", "i", false, "split squences according to sequence ID")
	splitCmd.Flags().StringP("by-region", "r", "", "split squences according to subsequence of given region. "+
		`e.g 1:12 for first 12 bases, -12:-1 for last 12 bases. type "fakit split -h" for more examples`)
	splitCmd.Flags().BoolP("md5", "m", false, "use MD5 instead of region sequence in output file when using flag -r (--by-region)")
	splitCmd.Flags().BoolP("two-pass", "2", false, "two-pass mode read files twice to lower memory usage. (only for FASTA format)")
	splitCmd.Flags().BoolP("dry-run", "d", false, "dry run, just print message and no files will be created.")
	splitCmd.Flags().BoolP("keep-temp", "k", false, "keep tempory FASTA and .fai file when using 2-pass mode")
}
