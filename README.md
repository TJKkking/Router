# DV Router

## 运行步骤
### 1. 编译运行
要求： golang version > 1.15 amd64
```shell
cd ./Router
go build
./router
```

### 2. Linux 下的Docker
```shell
# 从dockerhub拉取镜像（国内镜像源暂时未同步）
docker pull ootao/router:latest

# 运行镜像(-i 参数使用交互式命令行) 
docker run -i --name router_runtime ootao/router

# 进入stdin窗口，输入命令即可交互
# 例：使用router 2 2002 2003创建id为2，端口为2002，与端口为2003的路由器联通的路由器
router 2 2002 2003
```

### 3. Windows下的Docker
```shell
# Windows命令行拉取镜像
docker pull ootao/router:latest
# 图形化界面直接运行，进入容器Terminal

# 输入"./app"进入交互程序
./app
```
