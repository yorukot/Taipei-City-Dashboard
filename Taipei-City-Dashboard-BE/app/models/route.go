package models

type Stop struct {
	ID       int    `json:"-" gorm:"column:id;autoincrement;primaryKey"`
	StopUID  string `json:"stop_id" gorm:"column:stop_id;type:varchar;not null"`
	StopName string `json:"stop_name" gorm:"column:stop_name;type:varchar;not null"`
}

type Route struct {
	ID        int    `json:"-" gorm:"column:id;autoincrement;primaryKey"`
	RouteUID  string `json:"route_id" gorm:"column:route_id;type:varchar;not null"`
	RouteName string `json:"route_name" gorm:"column:route_name;type:varchar;not null"`
}

type StopRouteDataSource struct {
	ID          int    `json:"-" gorm:"column:id;autoincrement;primaryKey"`
	StopID      int    `json:"stop_id" gorm:"column:stop_id;not null"`
	RouteID     int    `json:"route_id" gorm:"column:route_id;not null"`
	DataSource  string `json:"data_source" gorm:"column:data_source;type:varchar;not null"`
	Description string `json:"description" gorm:"column:description;type:varchar;not null"`
	Stop        Stop   `gorm:"foreignKey:StopID"`
	Route       Route  `gorm:"foreignKey:RouteID"`
}

func GetAllRoutes() (routes []Route, err error) {
	err = DBManager.Find(&routes).Error
	return routes, err
}

func GetAllStopByRouteID(routeID int) (stops []Stop, err error) {
	err = DBManager.
		Joins("JOIN stop_route_data_sources ON stops.id = stop_route_data_sources.stop_id AND stop_route_data_sources.route_id = ?", routeID).
		Find(&stops).
		Error
	return stops, err
}
