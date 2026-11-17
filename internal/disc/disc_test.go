package disc

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
)

// Sizes and SHA-256s of every packed file of the Version A disc once unpacked, as produced by an
// independent ByteKiller decoder.
var unpacked = map[string]struct {
	size int
	sha  string
}{
	"ArcGrn1P": {30008, "84840dc156c1d5bf1ea6a7dcbe633615cbd7243aa4a3979846b4b65331a03241"},
	"ArcGrn2P": {42200, "a96255aeca0a2097528df7e90406268aac1012f0d0cfff76aadb1609f70098fa"},
	"ArcGrn3P": {30400, "dc330e456fbdfd45a02574757d899f0f3e32c7c6f7065962ab92b465ae838d5a"},
	"ArcGrn4P": {27008, "47bd62e2f68713bad78f20e061da9db2ad73dd877bcc40e217242eaf79075b20"},
	"ArcGrn5P": {40840, "59ada07015b02e94870c6b4d89baa28e0e01ca19e3c64fc65b53d73a7908026f"},
	"ArcObj1P": {48300, "495a2be974be4a50c50701e3056cf17261e05683da0e9bcb96c574d70ff6ee2e"},
	"ArcObj2P": {53580, "938ac2b6ba21f5e77a287be3877944371f94ffc9be890e9983660a9bd95da0e8"},
	"Arcobj3P": {47740, "d089e4cf009e596c04d165fd2d6b6e128a92338ca336c1fba26583df030fa551"},
	"ArcObj4P": {47420, "912edc7df57e6aba7ff833eb01913b12199159db7a070a2c3b7c64ad2b8648aa"},
	"Arcobj5P": {48530, "f098b430e11d6df2465bd05e496714a72eda2e1fab358d79bf546d5ba70799a3"},
	"Arcpan1p": {6400, "f6e55ad4744e28574ac516a7967507d45808b3546f7cae5eb1f22fcaff6fd9be"},
	"DmaLogo":  {32000, "27cf941ea1ad881944acaf38f0da48fcc39ceeadf5f39bc7c116e63ad1b043fa"},
	"Krisalis": {32000, "bb52044eab3e346c0b717824ef65eee2e1f4ef9b92d3e467353f916c801f4493"},
	"LemGrafP": {31744, "58541eca405aa2539d654de7a5fe0d795c25e5feaa2bcd10fa58ae71b5e4660c"},
	"LemProg":  {147264, "d5c15ccdb8241e767e64e47bf8727bb7138504fcad6c1344b3b47f2ace3f730e"},
	"LEVEL000": {8192, "8e4307bafd4e478d2324b0a202f3c8d71e8bc7c23fe0851a82b7ca184aa1346f"},
	"LEVEL001": {8192, "292b98f776523c7858cdfdeb52f045961ccfd5b309f901652af6502c96bc16f7"},
	"LEVEL002": {8192, "2729a5994574fcb4c9dc42f512e8d68faa0bd30270a0efc9eeffa699c8b48e9d"},
	"LEVEL003": {8192, "284bb1adeda47f3877656edf2518c2139be574003b36e49cfc49ada36849d0d5"},
	"LEVEL004": {8192, "57be27a30b7c6a3910313147f4dfb5702e191db20266dfefa1e3487ee46fb2e5"},
	"LEVEL005": {8192, "366482f87b005117a8160434a6e4d62ac3e0618529eac0fe681cc89c224c3ec6"},
	"LEVEL006": {8192, "0aad3582fa86f10a1bc47c5de93279075b1335023ca238a85dd61a69ee9ac5a6"},
	"LEVEL007": {8192, "16cc220fd8074a7c64cb95d52189c3e138c670434bb26303b84bd6ea35c42733"},
	"LEVEL008": {8192, "66c92e757326dc3190f10e93edbf7c62348bc3876c8429cc0937e4d2de528fb2"},
	"LEVEL009": {8192, "67cb20fbbad2e21bc4d8de567259ff7645e3793ef8ece11cb46f394aea568856"},
	"LEVEL010": {8192, "61c580544522b456d602b745925d344554225ce3139e0ec3a3673b313eab9d5d"},
	"LEVEL011": {8192, "c0be3ac4ec0b90609c084f513765d788f4565f4f0cd8abfc2cc737d7f643aaec"},
	"LEVEL012": {8192, "7301b01f302c7abc0507dcf18af541aa194b4f28b1b4e8a50457c8ef4c321ee2"},
	"LEVEL013": {8192, "4365c9a4d1a749afa3a674bcf20cecde7ee2586d2089e71bf49f1edef9296226"},
	"LEVEL014": {8192, "2f5a3fad8048442a7f8ea2f4ec0466b41b3d0df94ef4bff0a84f82f55c9d92a8"},
	"LEVEL015": {8192, "00ab51f6f534aa169c7b512e4e61016bb19eb3170b8d20be5d11111965081c96"},
	"LEVEL016": {8192, "98657b304bcd324dbc9b3add701eac22e539fd34a92304a0eb40199da16d3f13"},
	"LEVEL017": {8192, "88e13126c00c7f1f98acfefad509c4800d918fdac1dafdae2c657d12e5424df2"},
	"LEVEL018": {8192, "ce651c767eaa536476a1b0fa7b286827de9e3c48700414fe21c911299c5468bc"},
	"LEVEL019": {8192, "cb6aa30a5d12998153b13ea9720e9251319b2b6875fc0e9ee39f118fca2c1a56"},
	"LEVEL020": {8192, "cad1d7ba5f6b1f4bc2058c271d75235783d73099ea6ec2741a8f9e80cab30543"},
	"LEVEL021": {8192, "4e76d9be528d0ba904ccc8726eacb203c3bbec42edc538f08d7e040692970845"},
	"LEVEL022": {8192, "541962842fd31843a558338543ecb5dfcbe599e486b91e754fdabb9e7df55dbb"},
	"LEVEL023": {8192, "4c95ff5a740b164153c13fe77a6c0c7ac2324b6dd66dca65d31a5dbe165bd159"},
	"LEVEL024": {8192, "28c555cce23fe28679846a9dafdeba27028e14255bee60c7be21edbcb93a5d86"},
	"LEVELDAT": {9128, "4598f59b700182224c914b53e39bc4a23458f698e808d2b37bd20f39b37c636b"},
	"Psygno":   {32000, "4a6b90df8ed03569255d49e0b1c464091e752914b845fa0483b2981c86cbe574"},
	"SampdatP": {74234, "6b6e79c9c2641c0b18e4c9d056a7368cf9f90963d4b064d4b3009b70e1bb765b"},
	"SPECIAL0": {37640, "b30458d1841ac8f9fee077c73e471d17fab24a9588960a4660e2a8da5e2b365d"},
	"SPECIAL1": {26058, "f5784595b256e2bcb570274d21fddd1aecd28e25ed0f05f458b3e5bcc89ba4d2"},
	"SPECIAL2": {29204, "b3fc270c19cbb1e93e36ce707dbe231a4b883cb1e4a10bdb5c20b8ab89e2a8bd"},
	"SPECIAL3": {41588, "dad8e252ab9a729b4071c74887999d826c060a0270d440db246d849cd7a16c6e"},
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// readImage loads the disc image named by OPENLEMM_DISC.
func readImage(t *testing.T) []byte {
	t.Helper()
	name := os.Getenv("OPENLEMM_DISC")
	if name == "" {
		t.Skip("OPENLEMM_DISC not set")
	}
	img, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func readDisc(t *testing.T, img []byte) *Disc {
	t.Helper()
	d, err := Read(img)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestIdentifyVersionA(t *testing.T) {
	d := readDisc(t, readImage(t))
	if len(d.Files) != len(versionA) {
		t.Errorf("read %d files, Version A has %d", len(d.Files), len(versionA))
	}
	v, err := d.Identify()
	if err != nil || v != "A" {
		t.Fatalf("Identify = %q, %v; want A", v, err)
	}
}

func TestUnpackAll(t *testing.T) {
	d := readDisc(t, readImage(t))
	var got []string
	for p, data := range d.Files {
		out, err := Unpack(data)
		if err != nil {
			continue
		}
		name := path.Base(p)
		got = append(got, name)
		want, ok := unpacked[name]
		if !ok {
			t.Errorf("%s unpacked but is not a packed file", p)
			continue
		}
		if len(out) != want.size || sha(out) != want.sha {
			t.Errorf("%s: unpacked to %d bytes %s; want %d %s", p, len(out), sha(out), want.size, want.sha)
		}
	}
	if len(got) != len(unpacked) {
		slices.Sort(got)
		t.Errorf("unpacked %d files, want %d: %v", len(got), len(unpacked), got)
	}
}

func TestUnpackChecksum(t *testing.T) {
	d := readDisc(t, readImage(t))
	src := slices.Clone(d.Files["/!Lemmings/Lemmings/LemLevels/LEVEL018"])
	src[len(src)/2] ^= 0x10
	if _, err := Unpack(src); !errors.Is(err, errChecksum) {
		t.Fatalf("Unpack of corrupted stream: err = %v, want checksum mismatch", err)
	}
}

func TestDiscUnpack(t *testing.T) {
	d := readDisc(t, readImage(t))
	_, err := d.Unpack("/!Lemmings/Lemmings/NoSuchFile")
	if err == nil || !strings.Contains(err.Error(), "NoSuchFile: not on disc") {
		t.Errorf("missing file: err = %v, want not on disc", err)
	}
	const p = "/!Lemmings/Lemmings/LemProg"
	out, err := d.Unpack(p)
	if w := unpacked["LemProg"]; err != nil || len(out) != w.size || sha(out) != w.sha {
		t.Errorf("%s: %d bytes %s, %v; want %d %s", p, len(out), sha(out), err, w.size, w.sha)
	}
	d.Files[p] = d.Files[p][:len(d.Files[p])-4]
	if _, err := d.Unpack(p); err == nil || !strings.HasPrefix(err.Error(), p+": ") {
		t.Errorf("truncated %s: err = %v, want an error naming it", p, err)
	}
}

type stream struct {
	end  int // offset in the file of the byte after the stream's trailer
	data []byte
}

// embeddedStreams returns every ByteKiller stream in b whose trailer ends on a word boundary
// and whose checksum passes, in file order.
func embeddedStreams(b []byte) []stream {
	var found []stream
	for end := 12; end <= len(b); end += 4 {
		if data, err := Unpack(b[:end]); err == nil {
			found = append(found, stream{end, data})
		}
	}
	return found
}

// TestEmbeddedStreams scans the whole of LemProg: the three streams the menus unpack at fixed
// ends are the only ones there.
func TestEmbeddedStreams(t *testing.T) {
	d := readDisc(t, readImage(t))
	prog, err := Unpack(d.Files["/!Lemmings/Lemmings/LemProg"])
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		end  int
		size int
		sha  string
	}{
		{0xac18, 32000, "7d6a0b219e4c8d2d31812775484884e4ad2a6f8c5ea4df6363fa4a451dde998d"},
		{0xd7f8, 8704, "1add66f8baaa63e8d37265d559c9aa2e7d0b02792ab353bbe8e5382d92cbb50e"},
		{0xe948, 17472, "7c149bea34338a0a9afc415651864795e33d329efd0d8a2b657232b2ce236f81"},
	}
	got := embeddedStreams(prog)
	if len(got) != len(want) {
		for _, s := range got {
			t.Logf("found stream ending %#x, %d bytes", s.end, len(s.data))
		}
		t.Fatalf("found %d streams, want %d", len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.end != w.end || len(g.data) != w.size || sha(g.data) != w.sha {
			t.Errorf("stream %d: end %#x, %d bytes %s; want end %#x, %d bytes %s",
				i, g.end, len(g.data), sha(g.data), w.end, w.size, w.sha)
		}
	}
}

func TestIdentifyAlteredFile(t *testing.T) {
	img := slices.Clone(readImage(t))
	d := readDisc(t, img)
	for p, data := range d.Files {
		i := len(data) / 2
		data[i] ^= 0xff
		v, err := readDisc(t, img).Identify()
		data[i] ^= 0xff
		if err == nil || !strings.Contains(err.Error(), "unknown version") || !strings.Contains(err.Error(), p+" ") {
			t.Errorf("byte flipped in %s: Identify = %q, %v; want unknown version naming it", p, v, err)
		}
	}
}

// TestJFD's oracle is the player's disc as a JASPP JFD (OPENLEMM_JFD): rebuilt, it is Version A,
// every file hashing as versionA lists.
func TestJFD(t *testing.T) {
	name := os.Getenv("OPENLEMM_JFD")
	if name == "" {
		t.Skip("OPENLEMM_JFD not set")
	}
	img, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(img))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		img  []byte
	}{{"compressed", img}, {"uncompressed", raw}} {
		d := readDisc(t, c.img)
		if len(d.Files) != len(versionA) {
			t.Errorf("%s: read %d files, Version A has %d", c.name, len(d.Files), len(versionA))
		}
		if v, err := d.Identify(); err != nil || v != "A" {
			t.Errorf("%s: Identify = %q, %v; want A", c.name, v, err)
		}
	}
}
