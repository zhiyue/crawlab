package config

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestInitConfig(t *testing.T) {
	Convey("Test InitConfig func", t, func() {
		x := InitConfig("../conf/config.yml")

		Convey("The value should be nil", func() {
			So(x, ShouldEqual, nil)
		})
	})
}
