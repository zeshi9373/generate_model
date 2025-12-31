#使用说明

此自动生成model文件仅适配于项目 simplest_api、simplest_script，别的项目需要自行修改

复制配置文件 generateModel.json.template 为 generateModel.json

修改配置
```json
{
    "module_name":"simplest_api", // go项目module名称
    "output_dir":"./internal/model",
    "db_list":[
        {
            "pkg":"simplest",  // 对应的包名
            "link":"root:123456@tcp(127.0.0.1:3306)/simplest?charset=utf8mb4&parseTime=True&loc=Local", // 数据库链接
            "db_name":"simplest", // 数据库名称
            "table":[], // 针对设置的表生成model,不配置就是全量生成
            "const":"DBMain"  // 定义的常量 项目core包内定义
        }
    ]
}
```

### 生成可执行文件

linux

```
 go build .
```

windows

```
 GOOS=windows GOARCH=amd64 go build .
```