package level

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/benemon/openlemm/internal/disc"
)

// SHA-256 of each record's 1632×160 map image as RGB (row-major, 3 bytes per pixel, each 4-bit
// channel × 17), as an independent renderer produces them; that renderer is checked against the
// running game's grabs of all 120 levels.
var records = []struct {
	n   int
	sha string
}{
	{0, "d4795834d3c0244a0c1528d75a88f9a0a4cca24f9920fd99c761e46e4755ea7a"},
	{1, "424ad28cd8211e3142470df81131eceda37e3a110526d21acc17dad60c0d4c9c"},
	{2, "cfd38dfd8281cb45c015503eef593caa69d14c8467581949dd6e099502edf198"},
	{3, "cf862949f63b4a71c25074278af6ba2cce01c6862812efc6c5ab303b65aece53"},
	{4, "5b6473c81facd0882f6caf71be095052c8dd554ee5334a103131517bb3a88c19"},
	{5, "4fbbca37b70582414d50907171e16b658ffde344dd3b886a1a65720c29535503"},
	{6, "12c5b00866da705b593e83f211e8d14a92d6bb117135a5ff9fe4d21abadf75c8"},
	{7, "27eca79e736476a10c5de70a2e5ea02dec5cdf6055acfb64ed0485db2c9319ad"},
	{8, "0242171797afb8e74528ccf14792f08142c7eadb0fee07fb662c4bb2ca9383fe"},
	{9, "dd7ff8b611655ecbbbca9c81d3593c228254d5042cab76e810525593685ccb83"},
	{10, "d58ad7bb80397a9e9ae6dfe5ae7a9e091163855bc113050f965b0fdf20f9acbe"},
	{11, "8ba81ced52451f0c33b339d652226d39c6a1d788f240986abd8dbcd6b451784a"},
	{12, "382686b50db5700a94472a6c240cc7334fd09fa2760810c09d7cf2ab8547f65b"},
	{13, "5885315acc04d4509f7a4d529acc03d6b3d271cec04a8c61bee37e48889d56c5"},
	{14, "6d8be4e68620bc515b3ccbc42f6c8aa5ebd6f69b4c4c4d4ce445fa118eaa5d6d"},
	{15, "f4688e18af813d99ee24d4952160533d47d27ddd297cd52b50f3983a5ec42943"},
	{16, "0b1b527af512c3c8ee15fc8385f228a7a5bce49674a6e255aaca737bf2687c08"},
	{17, "3c0293e1091f8e3cc90160e06ce16a9b374e1e7b2f9aeb3a22ce63c28dfa6e04"},
	{18, "46bfd73f795dc9deac860b83db1dc8dbeb7baccb8b32e0372a11b68b9cee1561"},
	{19, "e7bb67dd1b4ba8d61bd07818a82122c6e48819f94216de8598cfef664f64a00e"},
	{20, "8434eb48444389e10518067614b9b2a5a1383af0521919604561278011755ca6"},
	{21, "e2f325d0373490de471ab1dacc3511eef4b3fd842780b794bbbc6b294941c459"},
	{22, "84272e3fbe6c4cdf06fd9e02d8ad0a0b04ccaa25e4cfb02adcb9dcf6eeff7d5c"},
	{23, "43c680877e82c3389c11a2332acbbd7dda3c5eb1c81bbd896421a3f1dd54e7b8"},
	{24, "ce74015c9b50f98acdacf7da717d20d43c55a5ca4675de481b57d7a4ca945ed3"},
	{25, "1a2a0bce30d94337098e47f03476478548c9e7ef8f9e4fefb22fb85cf6710e51"},
	{26, "ca540345367cfac468cbd4fb1f3300759295523cd487b63cc2073b0af7661192"},
	{27, "97ffe5ecbc083b7ee28c651f89d9f4155d23b4007638d577e49264b60c12a606"},
	{28, "c1356ee65c01c79ca73a96a5536f53f69e219362720a2dd4389a0c50344b9a39"},
	{29, "70b65a6138fb4ff42fcecff98433f4f267abd31a4a4be9060faca747eb232888"},
	{30, "89bdf0003a77469dea78bf494300a3b6a82b81e11b950cbf004d4f541d4811cb"},
	{31, "0a73f349569ba1184be1a9069e0824a53a88862914abd6c38c4416091def87b2"},
	{32, "d8b5a8834bff164937cfe1f3b72bb4f896a6abebb77a921798e84e0c673ea855"},
	{33, "16c2d64438e74393c0a0154cff85e747ce5cbd735c64a0049267b4ec89a940d7"},
	{34, "eec0c3af049d442cdd4ff2f8fe5d19a48ab83ff5b3483163e75d9f2a983ebf17"},
	{35, "03fc07968fb055b30d7f302a9faaa3e76b099b1329597625ca5192329af9917b"},
	{36, "8e9e42f798131af4960f4c070378877978627a77b12d8305311dd8fda79564af"},
	{37, "db14d156796e13524cd5d9cbeb8273b0551fc51b1a636e980eaef28e0ecdd682"},
	{38, "23e3015016bf3510bd20b560600ba313707745b8f3af02aeebcff9a730108506"},
	{39, "1ee046d1d59ebd0848b633e2eda94508050cc48f0c8d21370ae93e666ef348da"},
	{40, "1c1ac21a298da0e24f321349e89b186491b585bc83e9b8a6d92e9aa0873e0819"},
	{41, "9f054317e10c62062fab32b7dbfb598a3fa4a20314e46081130950deb4d65496"},
	{42, "36dfc625c2cb16b71754375b4bde8be3768e46acdd74bde294b9aa3b0ed83d02"},
	{43, "00e8685f34df9ea89b99480add1d28801b9d4db81b9b38e522c7b8eb1f7de2f1"},
	{44, "7069fbd45076fe9cc99ad511b91bf40e89ebb3eaf5943f3ea58fbb29118ca7a5"},
	{45, "e2252e03c0b67d336697fc969cf7c1349c310277b0e14df088bff72648195f86"},
	{46, "f79536ef069c578c67b324bab09a989b0db62268948fb73e9ddbd8e02ca9973a"},
	{47, "4258c3ed5a7fb44f22b3935748b173cb6d08c3a5ec8fe073f06038858cee8f5a"},
	{48, "f0658932e7141ae813c40e6ab3833fdb29b5eeeae74ea7abd1b5080c5350cc1b"},
	{49, "876bed347b522be71c02998f77c971796f13b7ab72e3c9b6ccf0c6556fe05ef8"},
	{50, "fbfc33771d957ff5ad454592ff1f51e29527b20768faa3b1b115d99099a6c82b"},
	{51, "c49b11c55814da12b517f1265f8e2735e36c6ae0daeaa2fec895806cff90fba5"},
	{52, "63adcc43c17b0100f7ddc788a92b37c6bf2fe2f7a444eccb7d238bbc0c598ac3"},
	{53, "7d297b1d1d84d136ffb00703a1471ad7d171fe5cbdaf4ac30ad1274d10a523e3"},
	{54, "e3a61839c5d2027340cac778f8a8416f9c0d44abcaa9618025a407680262416f"},
	{55, "6d8be4e68620bc515b3ccbc42f6c8aa5ebd6f69b4c4c4d4ce445fa118eaa5d6d"},
	{56, "3716fc5c1cadf7411cab4931a0eb1eab6aa9d4ca36e20166cf6dc40fc0654e50"},
	{57, "9368a598eca4a723fc2b4346fbe3d1885c3a4de6e5af85082f19bdf614ac53a4"},
	{58, "68cdf8345ebc2775d7a96b67c8e26ccba8c4cdda428ef73c7511af651bc4ffb6"},
	{59, "59ce4eb05c06e668f2b4236d2c6ed0c7ac17d67a0d263099ccfe3aab0c4cb3d1"},
	{60, "3504b796835614dde6e8792ee598762fa063aed15e42db0b174937854f29de54"},
	{61, "4c0673a828bdfbff097ea9028de8b515f6dc641f4d893e0f779f002d1134958e"},
	{62, "8706e2777f651fbd69c9272de853c81bea2f95de05d0d19379bd4f24fef73fbd"},
	{63, "2c17429baa68e488f6fe611b4a94028f02480424cd1a2426b500fbaf76bd54fb"},
	{64, "a4929dab1cb0f560e3ac3d374e742f7c2a6d5b23f80a9cc864bf9d2d7b914fb8"},
	{65, "4e61be022a1362e86fa6907c6a052df45e560eb4fd8e81cf88b61046b7efaecb"},
	{66, "94c1a036550df4b02d895bde684c0f274fad97960f0ea77ee077e3bbf332cef5"},
	{67, "4ffcd16f43b54cb45ecb7300771b3dee990325bd78c333f2cf4cd08b985e4cdd"},
	{68, "5edb0bd75d7c777b5fed870acf1c2716e5a82d956844b5c712709a45b0166405"},
	{69, "c76d5d8a6d655fab27e49e57927e521cbca4251ec1e6dd6e900b318132df8ec2"},
	{70, "23dd6f1f30ac5dacae5f1d935f502db6211208b5ce6d1b29705b4e80a9c436d9"},
	{71, "f8e7e527ed94110b89660b99a0a944d7c65ebd3bc2ad430af30629fdbab7c52e"},
	{72, "fdd93d119c843de89436d5c9526c1cc77bbaab1baa7710a8649c48a4b357bfd2"},
	{73, "04e05df00c15c7a4871d69bf580cc4e4ee2f031812b1b0abeea92fd5601d1c91"},
	{74, "20857d9dcbae67343ecd5fcf5f03e1403579b152ce7319b6c09b86f3b8a42fbc"},
	{75, "f7001a647d43809ddf04afe66d4bd669349f4d593a64ac3436c8770f25934e0c"},
	{76, "5ee15a08afcad7a81de52d7649a831b26dc0a469be6fcd0dcebc05f630c07f4a"},
	{77, "5dcf06df9acadb4bf7a5036d88d532dec8189c3867995f4521db047ad97641f1"},
	{78, "f05126904f8ef578b8596457c76570c22a0eefe2ab1bb886058b5549d599a8a3"},
	{79, "17ed80b14a31fcfaf3d04b49a60c43b9ea685feecc7ea13ef3bd7e585276c034"},
	{80, "b2acba7d53eecc487ce1caf6332ee647eb2342ad4aefc30e65259b6e0f3f85d4"},
	{81, "1cf5ca83afea3e6a9f76d75f79df383ee1c984f0c3f303f8e83027c2223a070c"},
	{82, "a9498566a805e458ba7c019236d67cf26f0b7c6b88783389291a20d7d979ff66"},
	{83, "bcfc07a73f4ece3c96931ab6dee70c56765937bee7cac12e0765a9660a9ff29e"},
	{84, "5e8d99e38ec7307172a3e807b44cadec085d81e3da8166291e33b1dac3657b2a"},
	{85, "bf6c11c184a7643590e4fc2d9eea3ee7ad5f8309d97cf080db770d573d59e530"},
	{86, "05589d10abe8c5e8d6fd2d47cd2d6ca7a2fae61d6079cbb97211410f3ad5dbca"},
	{87, "826ce8b13fe3037b865f1a98d346ba8afb34435d9445b0e60e44608901e630a0"},
	{88, "b1939cbf0b0343b4113326d4889a10fb4a40e6f582d65075330d6fe376519360"},
	{89, "2ea5220456f5fd354b6dbc4c1130781322bb2c2ff08cf2a7ac1d8a6a3ed1e2ee"},
	{90, "19efc80ae6642e4f70b4fa3486699d62f47f97196784c0c2a4332059dcb09612"},
	{91, "d224032f450805a0f4655395a87a5e64e313acd58433da94f419c84865efd177"},
	{92, "4db450e451fa529a2246187bfbdc4d9be354223cf10ada2d49fa154543255fbd"},
	{93, "7690be9eae670c08ce06e8f572c07b3cfbbee13311b79141bdb142970e538c49"},
	{94, "8f1cd690a78c013fb9396413c240d1394dea40535807471d8e2ffe136a807c37"},
	{95, "724600e0eab5a2fa1c2491bd80383c23cc68a44d75b527aec107448335d68d62"},
	{96, "f545ea619d5f109eda051666eebe61c81b5e50900c93af31347bcc063084619c"},
	{97, "7cd5847e3e8474f19fbed52bad7fce49ebaf6336b89285ed62e8f57b8532a1cb"},
	{98, "7f662cce41c52148fc3660840f93fcb988c386858ac1c1228c6041f23936e6bf"},
	{99, "1f8cc5f72de21b7350c76b759f7a76a412eaed9365911c54e67d66203561ab64"},
}

// readDisc reads the disc image named by OPENLEMM_DISC.
func readDisc(t *testing.T) *disc.Disc {
	t.Helper()
	name := os.Getenv("OPENLEMM_DISC")
	if name == "" {
		t.Skip("OPENLEMM_DISC not set")
	}
	img, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	d, err := disc.Read(img)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestRecords(t *testing.T) {
	d := readDisc(t)
	if len(records) != 100 {
		t.Fatalf("%d records in table, want 100", len(records))
	}
	for _, r := range records {
		l, err := Load(d, r.n)
		if err != nil {
			t.Errorf("record %d: %v", r.n, err)
			continue
		}
		rgb := make([]byte, 0, len(l.Image.Pix)*3)
		for _, v := range l.Image.Pix {
			c, g, b, _ := l.Image.Palette[v].RGBA()
			rgb = append(rgb, byte(c>>8), byte(g>>8), byte(b>>8))
		}
		s := sha256.Sum256(rgb)
		if got := hex.EncodeToString(s[:]); got != r.sha {
			t.Errorf("record %d: image %s, want %s", r.n, got, r.sha)
		}
	}
}

// TestFun1Header checks record 73 (Fun 1) against the pre-level screen and panel the running game
// shows for it: 10 lemmings, 10% to be saved, release rate 50, 5 minutes, 10 diggers and no other
// skills, initial view at x 624.
func TestFun1Header(t *testing.T) {
	l, err := Load(readDisc(t), 73)
	if err != nil {
		t.Fatal(err)
	}
	got := [5]int{l.Lemmings, l.ToSave, l.ReleaseRate, l.Minutes, l.StartX}
	if want := [5]int{10, 1, 50, 5, 624}; got != want {
		t.Errorf("lemmings, to save, release rate, minutes, start x = %v, want %v", got, want)
	}
	if want := [8]int{0, 0, 0, 0, 0, 0, 0, 10}; l.Skills != want {
		t.Errorf("skills = %v, want %v", l.Skills, want)
	}
}

// tune is one tune of a ProTracker module: it is entered at order positions first to loop, and
// the pattern at position last ends it with a B (position jump) to loop.
type tune struct{ first, loop, last int }

// tunes splits a ProTracker module's order list into tunes at its B effects.
func tunes(mod []byte) []tune {
	var ts []tune
	first := 0
	for p, pat := range mod[952 : 952+int(mod[950])] {
		for c := range 64 * 4 {
			if cell := mod[1084+1024*int(pat)+4*c:]; cell[2]&15 == 0xB {
				ts = append(ts, tune{first, int(cell[3]), p})
				first = p + 1
				break
			}
		}
	}
	return ts
}

// TestMusic's oracle is the disc's MUSINFO, LEMMUS0 and LEMMUS1: each module's tunes loop within
// themselves; the module-0 levels enter LEMMUS0's tunes in order, repeating every len(tunes)
// levels; the other levels enter each LEMMUS1 tune once; the game takes the level number modulo
// MUSINFO's count, 127.
func TestMusic(t *testing.T) {
	d := readDisc(t)
	var ts [2][]tune
	for m := range ts {
		mod := d.Files[fmt.Sprintf("%sLEMMUS%d", levelDir, m)]
		if len(mod) < 1084 || string(mod[1080:1084]) != "M.K." {
			t.Fatalf("LEMMUS%d: not a ProTracker M.K. module", m)
		}
		ts[m] = tunes(mod)
		for k, tu := range ts[m] {
			if tu.loop < tu.first || tu.loop > tu.last {
				t.Errorf("LEMMUS%d tune %d: positions %#x-%#x jump to %#x", m, k, tu.first, tu.last, tu.loop)
			}
		}
	}
	var ones, oneLevels []int
	for n := range 120 {
		m, s, err := Music(d, n)
		if err != nil {
			t.Fatal(err)
		}
		in := func(tu tune) bool { return s >= tu.first && s <= tu.loop }
		switch m {
		case 0:
			if k := n % len(ts[0]); !in(ts[0][k]) {
				t.Errorf("level %d: LEMMUS0 position %#x, want in tune %d, %#x-%#x", n, s, k, ts[0][k].first, ts[0][k].loop)
			}
		case 1:
			k := slices.IndexFunc(ts[1], in)
			if k < 0 {
				t.Errorf("level %d: LEMMUS1 position %#x enters no tune", n, s)
			}
			ones = append(ones, k)
			oneLevels = append(oneLevels, n)
		default:
			t.Errorf("level %d: module %d", n, m)
		}
	}
	// Fun 22, Tricky 14, Taxing 15 and Mayhem 22, as MUSINFO gives them.
	if !slices.Equal(oneLevels, []int{21, 43, 74, 111}) {
		t.Errorf("LEMMUS1 levels %v, want [21 43 74 111]", oneLevels)
	}
	slices.Sort(ones)
	once := len(ones) == len(ts[1])
	for i, k := range ones {
		once = once && k == i
	}
	if !once {
		t.Errorf("LEMMUS1 tunes entered %v of %d, want each once", ones, len(ts[1]))
	}
	m0, s0, _ := Music(d, 0)
	if m, s, _ := Music(d, 127); m != m0 || s != s0 {
		t.Errorf("level 127: (%d, %#x), want level 0's (%d, %#x)", m, s, m0, s0)
	}
}

// TestPlayIndex's oracle is LEVELDAT's level-order table: Fun 1 is record 73, and the play indexes
// 1-6 of the oracle fixtures are records 77, 78, 74, 75, 76 and 79.
func TestPlayIndex(t *testing.T) {
	d := readDisc(t)
	for want, rec := range []int{73, 77, 78, 74, 75, 76, 79} {
		if n, err := PlayIndex(d, rec); err != nil || n != want {
			t.Errorf("record %d: play index %d (%v), want %d", rec, n, err, want)
		}
	}
}

// TestTrapSounds's oracle is the trap sound rule: the traps in the five tilesets name samples 2, 3,
// 4, 5, 6, 7 and 9, at pitches &08, &0A, &10 and &11. Only placed traps are read, so a trap no
// level places is not seen.
func TestTrapSounds(t *testing.T) {
	d := readDisc(t)
	samples, pitches := map[int]bool{}, map[int]bool{}
	for rec := range 100 {
		l, err := Load(d, rec)
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range l.Objects {
			if o.Effect == 4 {
				samples[o.Sample], pitches[o.Pitch] = true, true
			}
		}
	}
	for s := range samples {
		if !map[int]bool{2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 9: true}[s] {
			t.Errorf("trap sample %d", s)
		}
	}
	for p := range pitches {
		if !map[int]bool{8: true, 10: true, 16: true, 17: true}[p] {
			t.Errorf("trap pitch %#x", p)
		}
	}
}

// TestLoadPlay's oracle is the 120 whole-screen fixtures grabbed from the running game at each
// level's start (OPENLEMM_FIXTURES/screens): each names the record whose terrain the level shows
// and the panel's minimum release rate and skill counts, which for the 40 OddTable levels come from
// the OddTable header, not the record's. The preview frame of every placed object is 1 for a
// trapdoor and 0 otherwise.
func TestLoadPlay(t *testing.T) {
	d := readDisc(t)
	dir := os.Getenv("OPENLEMM_FIXTURES")
	if dir == "" {
		t.Skip("OPENLEMM_FIXTURES not set")
	}
	odd := 0
	for n := range 120 {
		b, err := os.ReadFile(filepath.Join(dir, "screens", fmt.Sprintf("L%03d.json", n)))
		if err != nil {
			t.Fatal(err)
		}
		var f struct {
			Record int
			Panel  struct{ Buttons [10]int }
		}
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatal(err)
		}
		l, err := LoadPlay(d, n)
		if err != nil {
			t.Fatalf("play %d: %v", n, err)
		}
		r, err := Load(d, f.Record)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(l.Map.Pix, r.Map.Pix) {
			t.Errorf("play %d: terrain differs from record %d", n, f.Record)
		}
		if got := append([]int{l.ReleaseRate, l.ReleaseRate}, l.Skills[:]...); !slices.Equal(got, f.Panel.Buttons[:]) {
			t.Errorf("play %d: rate and skills %v, fixture panel %v", n, got, f.Panel.Buttons)
		}
		if l.Title != r.Title {
			odd++
		}
		if len(l.Title) != 32 {
			t.Errorf("play %d: title %q, want 32 characters", n, l.Title)
		}
		for _, o := range l.Objects {
			if want := map[bool]int{true: 1, false: 0}[o.Entrance]; o.Preview != want {
				t.Errorf("play %d: object id %d preview frame %d, want %d", n, o.ID, o.Preview, want)
			}
		}
	}
	if odd != 40 {
		t.Errorf("%d levels titled from OddTable, want 40", odd)
	}
}
