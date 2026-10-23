package proj

import (
	"testing"
)

// proj_internal_test.go — 内部路径覆盖:nil/错误分支、注册表 panic、
// EPSG 特例(900913/4269)、CRS() 绑定回传。

func TestStringNilCRS(t *testing.T) {
	var c *CRS
	if s := c.String(); s != "" {
		t.Errorf("nil CRS.String() 应为空串, got %q", s)
	}
}

func TestNewProjectionNilCRS(t *testing.T) {
	if _, err := (*CRS)(nil).NewProjection(); err == nil {
		t.Errorf("nil CRS NewProjection 应报错")
	}
	if _, err := (&CRS{}).NewProjection(); err == nil {
		t.Errorf("零值 CRS NewProjection 应报错")
	}
}

func TestNewLatlongNil(t *testing.T) {
	if _, err := newLatlong(nil); err == nil {
		t.Errorf("newLatlong(nil) 应报错")
	}
}

func TestNewEqcNil(t *testing.T) {
	if _, err := newEqc(nil); err == nil {
		t.Errorf("newEqc(nil) 应报错")
	}
}

func TestNewGeosNil(t *testing.T) {
	if _, err := newGeos(nil); err == nil {
		t.Errorf("newGeos(nil) 应报错")
	}
}

func TestCRSBindingIdentity(t *testing.T) {
	for _, crs := range []string{
		"+proj=eqc +lat_ts=0 +lon_0=180 +ellps=WGS84",
		"+proj=geos +h=35785863 +ellps=WGS84 +sweep=y",
	} {
		c, _ := Parse(crs)
		p, err := c.NewProjection()
		if err != nil {
			t.Fatalf("%s: %v", crs, err)
		}
		if p.CRS() != c {
			t.Errorf("%s: CRS() 未回传绑定实例", crs)
		}
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("重复注册应 panic")
		}
		// 查重必须先于写入:失败的注册不得污染全局注册表,否则运行期调用
		// register 的测试会与并发的 NewProjection 读构成 data race。
		if _, err := Parse("+proj=latlong +ellps=WGS84"); err != nil {
			t.Errorf("重复注册 panic 后注册表被污染: %v", err)
		}
	}()
	register("latlong", newLatlong) // latlong 已在 impl_latlong.go init 注册
}

func TestRegisterEmptyNamePanics(t *testing.T) {
	defer func() {
		if s := recover(); s == nil {
			t.Errorf("空名注册应 panic")
		} else {
			t.Logf("panic 文案: %v", s)
		}
	}()
	register("", newLatlong)
}

func TestEPSG900913(t *testing.T) {
	c, err := Parse("EPSG:900913")
	if err != nil {
		t.Fatalf("EPSG:900913: %v", err)
	}
	if c.EPSG() != 3857 {
		t.Errorf("900913 应常规化为 3857, got %d", c.EPSG())
	}
	s := c.String()
	c2, err := Parse(s)
	if err != nil || c2.EPSG() != 3857 {
		t.Errorf("900913 规范串往返: %q → %v %d", s, err, c2.EPSG())
	}
}

func TestEPSG4269(t *testing.T) {
	c, err := Parse("EPSG:4269")
	if err != nil {
		t.Fatalf("EPSG:4269: %v", err)
	}
	if c.EPSG() != 4269 || c.Proj != "latlong" {
		t.Errorf("4269 形态: proj=%s epsg=%d", c.Proj, c.EPSG())
	}
}

func TestEllipsoidInvalidCombos(t *testing.T) {
	cases := []string{
		"+proj=merc +rf=0.5",       // rf ∈ (0,1) 不合理
		"+proj=merc +rf=298.26",    // 孤立 rf(缺 +a),PROJ 拒收
		"+proj=merc +a=5 +f=1.2",   // f 出界
		"+proj=merc +a=5 +es=-0.1", // es 出界
		"+proj=merc +a=5 +b=6",     // b > a
		"+proj=merc +ellps=不存在",    // 未知椭球名
		"+proj=merc +R=-1",         // 负半径
		"+proj=merc +a=0",          // 非正规长半轴
		"+proj=merc +b=6",          // 孤立 b(缺 +a)
		"+proj=merc +es=0.1",       // 孤立 es(缺 +a)
		"+proj=merc +f=0.001",      // 孤立 f(缺 +a)
	}
	for _, s := range cases {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) 应报错", s)
		}
	}
}
