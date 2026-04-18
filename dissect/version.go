package dissect

const (
	Y7S1        int = 6884476
	Y7S2        int = 7040830
	Y7S4        int = 7338571
	Y8S1        int = 7408213
	Y8S2        int = 7601998
	Y8S3        int = 7762708
	Y8S4        int = 7921866
	Y9S1        int = 8111697
	Y9S1Update3 int = 8211379
	Y9S2        int = 8303162
	Y9S3        int = 8506016
	Y9S4        int = 8673114
	Y10S1       int = 8825661
	Y10S1_1     int = 8863180
	Y10S1_2     int = 8882422
	Y10S1_3     int = 8908078
	Y10S2_1     int = 9034019
	Y10S2_1_1   int = 9058361
	Y10S2_2     int = 9077538
	Y10S2_3     int = 9098584
	Y10S2_4     int = 9124272
	Y10S2_5     int = 9158643
	Y10S3       int = 9199003
	Y10S3_1     int = 9211553

	// Y11 constants below are observed from real replay headers rather than
	// lifted from an upstream release list. Add more patch builds (Y11S1_1,
	// Y11S2, ...) as replays for them come in. Nothing in the parser
	// currently gates on Y11 specifically — these exist so GameVersion
	// strings like "Y11S1_Alpha03" are no longer orphan numeric codes in
	// logs and so future season-gated logic has a named anchor.
	Y11S1_Alpha03 int = 9625601
)
