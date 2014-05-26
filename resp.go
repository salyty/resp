package resp

import (
	"io"
	"errors"
	"bytes"
	"strconv"
	"strings"
)

const (
	T_SimpleString = '+'
	T_Error	   = '-'
	T_Integer	   = ':'
	T_BulkString  = '$'
	T_Array	   = '*'
)

var CRLF = []byte{'\r', '\n'}

//Command
//
//Command 格式：Inline Command 与 Array With BulkString
type Command struct {
	//根据惯例，Args[0] 是Name本身
	Args []string
}

//返回Command的名称，如GET\SET
func (c Command) Name() string {
	if len(c.Args)==0 {
		return ""
	} else {
		return c.Args[0]
	}
}

//以String形式获取Command[index]
func (c Command) String(index int) (ret string) {
	if len(c.Args) > index {
		ret = c.Args[index]
	}
	return ret
}

//以int64的形式返回Command.Args[index]
func (c Command) Integer(index int) (ret int64) {
	if len(c.Args) > index {
		ret, _ = strconv.ParseInt(c.Args[index], 10, 64)
	}
	return ret
}

//统一格式化为ArrayWithBulkString
func (c Command) Format() []byte {
	var ret *bytes.Buffer
	ret = new(bytes.Buffer)

	ret.WriteByte(T_Array)
	ret.WriteString(strconv.Itoa(len(c.Args)))
	ret.Write(CRLF)
	for index := range c.Args {
		ret.WriteByte(T_BulkString)
		ret.WriteString(strconv.Itoa(len(c.Args[index])))
		ret.Write(CRLF)
		ret.WriteString(c.Args[index])
		ret.Write(CRLF)
	}
	return ret.Bytes()
}

func NewCommand(args ...string) (*Command, error) {
	if len(args) == 0 {
		return nil, errors.New("err_new_cmd")
	}
	return &Command{Args:args}, nil
}

//从Reader中读取Command
func ReadCommand(r io.Reader) (*Command, error) {
	buf, err := readRespCommandLine(r)
	
	if nil != err && !(io.EOF == err && len(buf) > 1 ) {
		return nil, err
	}

	if T_Array != buf[0] {
		return NewCommand(strings.Fields(string(buf))...)
	}

	//Command: BulkString
	var ret *Data
	ret = new(Data)

	ret, err = readDataForSpecType(r, buf)
	if nil != err {
		return nil, err
	}

	commandArgs := make([]string, len(ret.Array))
	for index := range ret.Array {
		if ret.Array[index].T != T_BulkString {
			return nil, errors.New("Unexpected Command Type")
		}
		commandArgs[index] = string(ret.Array[index].String)
	}

	return NewCommand(commandArgs...)
}

type Data struct {
	T byte
	String []byte
	Integer int64
	Array []*Data
	IsNil bool
}

func (d Data) Format() []byte {
	var ret *bytes.Buffer
	ret = new(bytes.Buffer)

	ret.WriteByte(d.T)
	if d.IsNil {
		ret.WriteString("-1")
		ret.Write(CRLF)
		return ret.Bytes()
	}

	switch d.T {
		case T_SimpleString, T_Error:
			ret.Write(d.String)
			ret.Write(CRLF)
		case T_BulkString:
			ret.WriteString(strconv.Itoa(len(d.String)))
			ret.Write(CRLF)
			ret.Write(d.String)
			ret.Write(CRLF)
		case T_Integer:
			ret.WriteString(strconv.FormatInt(d.Integer, 10))
			ret.Write(CRLF)
		case T_Array:
			ret.WriteString(strconv.Itoa(len(d.Array)))
			ret.Write(CRLF)
			for index := range d.Array {
				ret.Write(d.Array[index].Format())
			}
	}
	return ret.Bytes()
}

func ReadData(r io.Reader) (*Data, error) {
	buf, err := readRespLine(r)
	if nil != err {
		return nil, err
	}

	if len(buf) < 2 {
		return nil, errors.New("invalid Data Source: " + string(buf))
	}

	return readDataForSpecType(r, buf)
}

func readDataForSpecType(r io.Reader, line []byte) (*Data, error) {

	var err error
	var ret *Data

	ret = new(Data)
	switch line[0] {
		case T_SimpleString:
			ret.T = T_SimpleString
			ret.String = line[1:]

		case T_Error:
			ret.T = T_Error
			ret.String = line[1:]

		case T_Integer:
			ret.T = T_Integer
			ret.Integer, err = strconv.ParseInt(string(line[1:]), 10, 64)

		case T_BulkString:
			var lenBulkString int64
			lenBulkString, err = strconv.ParseInt(string(line[1:]), 10, 64)

			ret.T = T_BulkString
			if -1 != lenBulkString {
				ret.String, err = readRespN(r, lenBulkString)
				_, err = readRespN(r, 2)
			} else {
				ret.IsNil = true
			}

		case T_Array:
			var lenArray int64
			var i int64
			lenArray, err = strconv.ParseInt(string(line[1:]), 10, 64)

			ret.T = T_Array
			if nil==err {
				if -1 != lenArray {
					ret.Array = make([]*Data, lenArray)
					for i=0; i<lenArray && nil == err; i++ {
						ret.Array[i], err = ReadData(r)
					}
				} else {
					ret.IsNil = true
				}
			}

		default: //Maybe you are Inline Command
			err = errors.New("Unexpected type ")

	}
	return ret, err
}




//读取当前行，并去掉最后的\r\n
func readRespLine(r io.Reader) ([]byte, error) {

	var i int
	var err error
	var buf []byte
	var ret *bytes.Buffer

	buf = make([]byte, 1)
	ret = &bytes.Buffer{}

	for {
		_, err = io.ReadFull(r, buf)
		if nil != err {
			return nil, err
		}

		i++
		ret.WriteByte(buf[0])
		if '\n' == buf[0] {
			break
		}
	}

	return ret.Next(i-2), nil
}

//读取老的redis协议 InlineCommand
func readRespCommandLine(r io.Reader) ([]byte, error) {

	var err error
	var buf []byte
	var ret *bytes.Buffer

	buf = make([]byte, 1)
	ret = &bytes.Buffer{}

	for {
		_, err = io.ReadFull(r, buf)
		if nil != err {
			if io.EOF == err {
				break
			}
			return nil, err
		}

		ret.WriteByte(buf[0])
		if '\n' == buf[0] {
			break
		}
	}

	return bytes.TrimSpace(ret.Bytes()), err
}


//读取N个字节，并去掉最后的\r\n
func readRespN(r io.Reader, n int64) ([]byte, error) {
	var err error
	var ret []byte

	ret = make([]byte, n)
	_, err = io.ReadFull(r, ret)
	if nil!=err {
		ret = nil
	}
	return ret, err
}

//读取当前行的数字，并去掉最后的\r\n
func readRespIntLine(r io.Reader) (int64, error) {
	line, err := readRespLine(r)
	if nil!=err {
		return 0, err
	}
	return strconv.ParseInt(string(line), 10, 64)
}
