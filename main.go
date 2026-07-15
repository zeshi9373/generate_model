package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"

	_ "github.com/go-sql-driver/mysql" // MySQL驱动
)

type ListItem struct {
	Pkg    string   `json:"pkg"`
	Link   string   `json:"link"`
	DBName string   `json:"db_name"`
	Table  []string `json:"table"`
	Const  string   `json:"const"`
}

// 配置
type Config struct {
	DbLisit    []ListItem `json:"db_list"`
	OutputDir  string     `json:"output_dir"`
	ModuleName string     `json:"module_name"`
}

// 列信息
type ColumnInfo struct {
	ColumnName    string
	DataType      string
	IsNullable    string
	ColumnKey     string
	ColumnDefault sql.NullString
	ColumnComment string
	Extra         string
}

// 表信息
type TableInfo struct {
	TableName string
	Columns   []ColumnInfo
}

// 生成Model的结构
type ModelTemplateData struct {
	PackageName string
	TableName   string
	StructName  string
	Imports     []string
	Columns     []ColumnField
	Const       string
}

// 列字段信息
type ColumnField struct {
	FieldName string
	GormTag   string
	JsonTag   string
	GoType    string
	Comment   string
}

func main() {
	byteStream, err := os.ReadFile("./generateModel.json")

	if err != nil {
		panic("read config file error")
	}

	c := Config{}
	if err := json.Unmarshal(byteStream, &c); err != nil {
		panic("unmarshal config file error")
	}

	for _, item := range c.DbLisit {
		// 获取所有表
		tables, err := getTables(item)
		if err != nil {
			log.Fatal("Failed to get tables:", err)
		}

		// 为每个表生成Model
		for _, table := range tables {
			if len(item.Table) > 0 && !IsInSlice(item.Table, table) {
				continue
			}

			if err := generateModelForTable(item, table, c); err != nil {
				log.Printf("Failed to generate model for table %s: %v", table, err)
				continue
			}
			fmt.Printf("Generated model for db,table: %s,%s\n", item.DBName, table)
		}
	}

}

// 获取数据库中的所有表
func getTables(config ListItem) ([]string, error) {
	db, err := sql.Open("mysql", config.Link)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query("SHOW TABLES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		tables = append(tables, tableName)
	}

	return tables, nil
}

// 为指定表生成Model文件
func generateModelForTable(config ListItem, tableName string, c Config) error {
	db, err := sql.Open("mysql", config.Link)
	if err != nil {
		return err
	}
	defer db.Close()

	// 获取列信息
	columns, err := getColumns(db, config.DBName, tableName)
	if err != nil {
		return err
	}

	// 准备模板数据
	data := prepareTemplateData(tableName, columns, config, c)
	// 创建输出目录
	if err := os.MkdirAll(c.OutputDir+"/"+config.Pkg, 0755); err != nil {
		return err
	}

	// 生成文件名
	outputFile := fmt.Sprintf("%s/%s.go", c.OutputDir+"/"+config.Pkg, toSnakeCase(tableName))

	// 写入文件
	return writeModelFile(outputFile, data)
}

// 获取表的列信息
func getColumns(db *sql.DB, dbName, tableName string) ([]ColumnInfo, error) {
	query := `
		SELECT 
			COLUMN_NAME, 
			DATA_TYPE, 
			IS_NULLABLE, 
			COLUMN_KEY,
			COLUMN_DEFAULT,
			COLUMN_COMMENT,
			EXTRA
		FROM INFORMATION_SCHEMA.COLUMNS 
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION
	`

	rows, err := db.Query(query, dbName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		if err := rows.Scan(
			&col.ColumnName,
			&col.DataType,
			&col.IsNullable,
			&col.ColumnKey,
			&col.ColumnDefault,
			&col.ColumnComment,
			&col.Extra,
		); err != nil {
			return nil, err
		}
		columns = append(columns, col)
	}

	return columns, nil
}

// 准备模板数据
func prepareTemplateData(tableName string, columns []ColumnInfo, item ListItem, c Config) ModelTemplateData {
	var cols []ColumnField
	var isTimePkg bool
	// 处理每个列
	for _, col := range columns {
		goType := mysqlTypeToGoType(col.DataType, col.IsNullable)

		if !isTimePkg && strings.Contains(goType, "time.") {
			isTimePkg = true
		}

		field := ColumnField{
			FieldName: toCamelCase(col.ColumnName),
			GormTag:   buildGormTag(col),
			// JsonTag:   buildJsonTag(col.ColumnName),
			JsonTag: col.ColumnName,
			GoType:  goType,
			Comment: col.ColumnComment,
		}
		cols = append(cols, field)
	}

	data := ModelTemplateData{
		PackageName: item.Pkg,
		TableName:   tableName,
		StructName:  toCamelCase(tableName),
		Columns:     cols,
		Imports:     []string{c.ModuleName + "/core", c.ModuleName + "/core/svc", "gorm.io/gorm"},
		Const:       item.Const,
	}

	if isTimePkg {
		data.Imports = append(data.Imports, "time")
	}

	return data
}

// 构建Gorm标签
func buildGormTag(col ColumnInfo) string {
	tags := []string{"column:" + col.ColumnName}

	if col.ColumnKey == "PRI" {
		tags = append(tags, "primary_key")
	}

	if col.Extra == "auto_increment" {
		tags = append(tags, "AUTO_INCREMENT")
	}

	if col.IsNullable == "NO" {
		tags = append(tags, "NOT NULL")
	}

	if col.ColumnDefault.Valid && col.ColumnDefault.String != "" {
		if col.ColumnDefault.String == "CURRENT_TIMESTAMP" {
			tags = append(tags, "default:CURRENT_TIMESTAMP")
		} else {
			tags = append(tags, fmt.Sprintf("default:%s", col.ColumnDefault.String))
		}
	}

	return strings.Join(tags, ";")
}

// 构建JSON标签
func buildJsonTag(columnName string) string {
	return toCamelCaseLower(columnName)
}

// MySQL类型转Go类型
func mysqlTypeToGoType(mysqlType, isNullable string) string {
	goType := ""

	// 判断基本类型
	switch strings.ToLower(mysqlType) {
	case "tinyint", "smallint", "mediumint", "int":
		goType = "int"
	case "bigint":
		goType = "int64"
	case "float", "double":
		goType = "float64"
	case "decimal":
		goType = "decimal.Decimal"
	case "char", "varchar", "text", "tinytext", "mediumtext", "longtext":
		goType = "string"
	case "date", "datetime", "timestamp":
		goType = "string"
	case "time":
		goType = "string"
	case "year":
		goType = "int"
	case "bit":
		goType = "[]byte"
	case "binary", "varbinary", "blob", "tinyblob", "mediumblob", "longblob":
		goType = "[]byte"
	case "enum", "set":
		goType = "string"
	default:
		goType = "string"
	}

	// 处理可空字段
	// if isNullable == "YES" && !strings.HasPrefix(goType, "*") && goType != "[]byte" {
	// 	goType = "*" + goType
	// }

	return goType
}

// 下划线转驼峰（首字母大写）
func toCamelCase(s string) string {
	return toCamelCaseHelper(s, true)
}

// 下划线转驼峰（首字母小写）
func toCamelCaseLower(s string) string {
	return toCamelCaseHelper(s, false)
}

func toCamelCaseHelper(s string, firstUpper bool) string {
	parts := strings.Split(strings.ToLower(s), "_")
	result := ""

	for i, part := range parts {
		if i == 0 && !firstUpper {
			result += part
		} else {
			result += strings.Title(part)
		}
	}

	return result
}

// 驼峰转下划线
func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}

// 写入Model文件
func writeModelFile(filename string, data ModelTemplateData) error {
	// Model模板
	modelTemplate := `package {{.PackageName}}

import (
	{{- range .Imports}}
	"{{.}}"
	{{- end}}
)

// {{.StructName}} {{.TableName}}表的Model
type {{.StructName}} struct {
	{{- range .Columns}}
	{{.FieldName}} {{.GoType}} ` + "`gorm:\"{{.GormTag}}\" json:\"{{.JsonTag}}\"`" + ` {{if .Comment}}// {{.Comment}}{{end}}
	{{- end}}
}

// 配置信息
type {{.StructName}}Config struct {
	Db    string
	Table string
}

// 获取配置
func Get{{.StructName}}Config() {{.StructName}}Config {
	return {{.StructName}}Config{
		Db:    core.{{.Const}},
		Table: "{{.TableName}}",
	}
}

// 创建新的Model实例
func New{{.StructName}}Model() *gorm.DB {
	return svc.NewDb(Get{{.StructName}}Config().Db).Table(Get{{.StructName}}Config().Table)
}
`

	tmpl, err := template.New("model").Parse(modelTemplate)
	if err != nil {
		return err
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	return tmpl.Execute(file, data)
}

func IsInSlice(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}

	return false
}
