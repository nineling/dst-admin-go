package service

import (
	"crypto/rand"
	"dst-admin-go/config/database"
	"dst-admin-go/config/global"
	"dst-admin-go/constant/consts"
	"dst-admin-go/model"
	"dst-admin-go/utils/clusterUtils"
	"dst-admin-go/utils/dstUtils"
	"dst-admin-go/utils/fileUtils"
	"dst-admin-go/utils/shellUtils"
	"dst-admin-go/vo"
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
)

type ClusterManager struct {
	InitService
	HomeService
	s GameService
}

func (c *ClusterManager) QueryCluster(ctx *gin.Context) {
	//获取查询参数
	clusterName := ctx.Query("clusterName")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(ctx.DefaultQuery("size", "10"))
	if page <= 0 {
		page = 1
	}
	if size < 0 {
		size = 10
	}
	db := database.DB

	if clusterName, isExist := ctx.GetQuery("clusterName"); isExist {
		db = db.Where("cluster_name LIKE ?", "%"+clusterName+"%")
	}

	db = db.Order("created_at desc").Limit(size).Offset((page - 1) * size)

	clusters := make([]model.Cluster, 0)

	if err := db.Find(&clusters).Error; err != nil {
		fmt.Println(err.Error())
	}

	var total int64
	db2 := database.DB
	if clusterName != "" {
		db2.Model(&model.Cluster{}).Where("clusterName like ?", "%"+clusterName+"%").Count(&total)
	} else {
		db2.Model(&model.Cluster{}).Count(&total)
	}
	totalPages := total / int64(size)
	if total%int64(size) != 0 {
		totalPages++
	}

	var clusterVOList = make([]vo.ClusterVO, len(clusters))
	var wg sync.WaitGroup
	wg.Add(len(clusters))
	for i, cluster := range clusters {
		go func(cluster model.Cluster, i int) {
			clusterVO := vo.ClusterVO{
				ClusterName:     cluster.ClusterName,
				Description:     cluster.Description,
				SteamCmd:        cluster.SteamCmd,
				ForceInstallDir: cluster.ForceInstallDir,
				Backup:          cluster.Backup,
				ModDownloadPath: cluster.ModDownloadPath,
				Uuid:            cluster.Uuid,
				Beta:            cluster.Beta,
				ID:              cluster.ID,
				CreatedAt:       cluster.CreatedAt,
				UpdatedAt:       cluster.UpdatedAt,
				Master:          dstUtils.Status(cluster.ClusterName, "Master"),
				Caves:           dstUtils.Status(cluster.ClusterName, "Caves"),
			}
			clusterIni := c.GetClusterIni(cluster.ClusterName)
			name := clusterIni.ClusterName
			maxPlayers := clusterIni.MaxPlayers
			mode := clusterIni.GameMode
			password := clusterIni.ClusterPassword
			var hasPassword int
			if password == "" {
				hasPassword = 0
			} else {
				hasPassword = 1
			}
			// http 请求服务信息
			homeInfos := clusterUtils.GetDstServerInfo(name)
			if len(homeInfos) > 0 {
				for _, info := range homeInfos {
					if info.Name == name &&
						uint(info.MaxConnect) == maxPlayers &&
						info.Mode == mode &&
						int(info.Password) == hasPassword {
						clusterVO.RowId = info.Row
						clusterVO.Connected = int(info.Connected)
						clusterVO.MaxConnections = int(info.MaxConnect)
						clusterVO.Mode = info.Mode
						clusterVO.Mods = int(info.Mods)
						clusterVO.Season = info.Season
						clusterVO.Region = info.Region
					}

				}
			}
			clusterVOList[i] = clusterVO
			wg.Done()
		}(cluster, i)
	}
	wg.Wait()
	ctx.JSON(http.StatusOK, vo.Response{
		Code: 200,
		Msg:  "success",
		Data: vo.Page{
			Data:       clusterVOList,
			Page:       page,
			Size:       size,
			Total:      total,
			TotalPages: totalPages,
		},
	})

}

func (c *ClusterManager) CreateCluster(cluster *model.Cluster) {

	db := database.DB
	tx := db.Begin()

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	cluster.Uuid = generateUUID()
	err := db.Create(&cluster).Error

	if err != nil {
		if err.Error() == "Error 1062: Duplicate entry" {
			log.Panicln("集群名称重复，请更换另一个名字！！！")
		}
		log.Panicln("创建集群失败！")
	}

	// 安装 dontstarve_dedicated_server
	log.Println("正在安装饥荒。。。。。。")
	if !fileUtils.Exists(cluster.ForceInstallDir) {
		// app_update 343050 beta updatebeta validate +quit
		cmd := "cd " + cluster.SteamCmd + " ; ./steamcmd.sh +force_install_dir " + cluster.ForceInstallDir + " +login anonymous +app_update 343050 validate +quit"
		output, err := shellUtils.Shell(cmd)
		if err != nil {
			log.Panicln("饥荒安装失败")
		}
		log.Println(output)
	}
	log.Println("饥荒安装完成！！！")
	// 创建世界
	c.InitCluster(cluster, global.ClusterToken)

	tx.Commit()
}

func (c *ClusterManager) UpdateCluster(cluster *model.Cluster) {
	db := database.DB
	oldCluster := &model.Cluster{}
	db.Where("id = ?", cluster.ID).First(oldCluster)
	oldCluster.Description = cluster.Description
	//oldCluster.SteamCmd = cluster.SteamCmd
	//oldCluster.ForceInstallDir = cluster.ForceInstallDir
	db.Updates(oldCluster)
}

func (c *ClusterManager) DeleteCluster(id uint) (*model.Cluster, error) {

	db := database.DB
	cluster1 := model.Cluster{}
	db.Where("id = ?", id).First(&cluster1)

	if cluster1.ID == 0 {
		log.Panicln("删除集群失败, ID 不存在 ", id)
	}

	cluster := model.Cluster{}
	result := db.Where("id = ?", id).Unscoped().Delete(&cluster)

	if result.Error != nil {
		return nil, result.Error
	}

	log.Println("正在删除 cluster", cluster1)

	// 停止服务
	c.s.StopGame(cluster1.ClusterName, 0)

	// TODO 删除集群 和 饥荒、备份、mod 下载

	// 删除集群

	// 删除饥荒
	log.Println("正在删除集群: ", cluster1.ForceInstallDir)
	err := fileUtils.DeleteDir(cluster1.ForceInstallDir)
	if err != nil {
		return nil, err
	}

	clusterPath := filepath.Join(consts.KleiDstPath, cluster1.ClusterName)
	log.Println("正在删除存档: ", clusterPath)
	err = fileUtils.DeleteDir(clusterPath)
	if err != nil {
		return nil, err
	}

	return &cluster1, nil
}

func (c *ClusterManager) FindClusterByUuid(uuid string) *model.Cluster {
	db := database.DB
	cluster := &model.Cluster{}
	db.Where("uuid=?", uuid).First(cluster)
	return cluster
}

// 生成随机UUID
func generateUUID() string {
	// 生成随机字节序列
	var uuid [16]byte
	_, err := rand.Read(uuid[:])
	if err != nil {
		panic(err)
	}

	// 设置UUID版本和变体
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0xbf) | 0x80 // Variant 1

	// 将UUID转换为字符串并返回
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
}
