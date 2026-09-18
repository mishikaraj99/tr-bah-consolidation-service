package legacy

import (
	"context"
	"math"
	"time"

	"golang.org/x/sync/errgroup"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/orders"
	"traya-bah-service/models"
)

// BahBanner is the bahBanner payload.
type BahBanner struct {
	Title      string `json:"title"`
	SubTitle   string `json:"subTitle"`
	CtaText    string `json:"ctaText"`
	Coins      int    `json:"coins"`
	UnlockText string `json:"unlockText"`
}

// CoinDiscountCap is the coinDiscountCap payload.
type CoinDiscountCap struct {
	Value int    `json:"value"`
	Type  string `json:"type"`
}

// StreakAndRewardBalance is the GET /streakAndRewardBalance response.
type StreakAndRewardBalance struct {
	RewardBalance                    int               `json:"rewardBalance"`
	CurrentDaysStreakCount           int               `json:"currentDaysStreakCount"`
	LongestDaysStreakCount           int               `json:"longestDaysStreakCount"`
	ThreeDaysStreakCount             int               `json:"threeDaysStreakCount"`
	SevenDaysStreakCount             int               `json:"sevenDaysStreakCount"`
	TwentyOneDaysStreakCount         int               `json:"twentyOneDaysStreakCount"`
	LastLogDate                      time.Time         `json:"lastLogDate"`
	FirstLogDate                     time.Time         `json:"firstLogDate"`
	IsUserEligibleForNewBahFlow      bool              `json:"isUserEligibleForNewBahFlow"`
	BahBanner                        any               `json:"bahBanner"`
	BahTitle                         string            `json:"bahTitle"`
	PopupText                        PopupTextT        `json:"popupText"`
	CoinDiscountCap                  CoinDiscountCap   `json:"coinDiscountCap"`
	CoinConversionRatio              string            `json:"coinConversionRatio"`
	CoinNotApplied                   CoinInfo          `json:"coinNotApplied"`
	CoinApplied                      CoinInfo          `json:"coinApplied"`
	AutoApplyCoins                   bool              `json:"autoApplyCoins"`
	BahbannerNewTitle                string            `json:"bahbannerNewTitle"`
	StreakRewardMessage              string            `json:"streakRewardMessage"`
	HasUserSeenBahUpdatedModalResult bool              `json:"hasUserSeenBahUpdatedModalResult"`
	BannerWidgetData                 *BannerWidgetData `json:"bannerWidgetData"`
	ShowBAHLogMissedToUser           bool              `json:"showBAHLogMissedToUser"`
	Modals                           Modals            `json:"modals"`
	ReminderOnBahPage                string            `json:"reminderOnBahPage"`
}

// GetOnlyRewardBalance mirrors getOnlyRewardBalance (rounded active coin balance).
func (s *Service) GetOnlyRewardBalance(ctx context.Context, userID string) (int, error) {
	bal, err := s.Store.ActiveCoinBalance(ctx, userID, s.now())
	if err != nil {
		return 0, err
	}
	return int(math.Round(bal.BalanceCoins)), nil
}

// GetUserRewardBalance resolves a caseId (or, for the internal service route, a userId) to a balance.
func (s *Service) GetUserRewardBalance(ctx context.Context, caseOrUserID string) (int, error) {
	userID, err := s.PG.UserIDFromCaseID(ctx, caseOrUserID)
	if err != nil {
		return 0, err
	}
	if userID == "" {
		ok, err := s.PG.UserExists(ctx, caseOrUserID)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, common.BadRequest("Invalid case id")
		}
		userID = caseOrUserID
	}
	return s.GetOnlyRewardBalance(ctx, userID)
}

// ExpiringReward is getEarliestExpiringUnusedCoins' result.
type ExpiringReward struct {
	RemainingCoins int
	ExpiringOn     string
}

// GetEarliestExpiringUnusedCoins mirrors handler.js getEarliestExpiringUnusedCoins.
func (s *Service) GetEarliestExpiringUnusedCoins(ctx context.Context, userID string) (*ExpiringReward, error) {
	txn, err := s.Store.EarliestExpiringUnusedCredit(ctx, userID, common.UTCMidnight(s.now()))
	if err != nil || txn == nil || txn.ID.IsZero() {
		return nil, err
	}
	out := &ExpiringReward{RemainingCoins: txn.CreditCoins - txn.TotalDebitCoins}
	if txn.ExpireAt != nil {
		out.ExpiringOn = common.FormatMoment(*txn.ExpireAt, "YYYY-MM-DD")
	}
	return out, nil
}

// HasUserSeenBahUpdatedModal ports handler.js hasUserSeenbahUpdatedModal; only showBAHLogMissedToUser is used.
func (s *Service) HasUserSeenBahUpdatedModal(ctx context.Context, userID string, hasLoggedEver bool, lastLog *time.Time) (bool, error) {
	now := s.now()
	todayStart := common.StartOfDayUTC(now)
	todayEnd := common.EndOfDayUTC(now)
	latest, _, err := s.PG.LatestOrderAndCount(ctx, userID)
	if err != nil {
		return false, err
	}
	caseID := ""
	if latest != nil {
		caseID = latest.CaseID
	}
	dayDiff := 0
	if lastLog != nil && !lastLog.IsZero() {
		dayDiff = int(common.UTCMidnight(now).Sub(common.UTCMidnight(*lastLog)).Hours() / 24)
	}
	missedEmpty := true
	if caseID != "" {
		window := [2]time.Time{todayStart, todayEnd}
		missed, err := s.Store.FindCustomerActivityLog(ctx, caseID, BahMissedLogYesterdayEvt, &window)
		if err != nil {
			return false, err
		}
		missedEmpty = missed == nil || missed.Event == ""
	}
	return hasLoggedEver && dayDiff == 2 && missedEmpty, nil
}

// GetStreakAndRewardBalance ports handler.js getStreakAndRewardBalance.
func (s *Service) GetStreakAndRewardBalance(ctx context.Context, userID string, version int, authToken string) (*StreakAndRewardBalance, error) {
	now := s.now()
	var (
		balance  mongoBalance
		streak   *models.StreakLog
		txns     []models.RewardTransaction
		nonVoid  []orders.Order
		userCase *userCaseRef
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		b, err := s.Store.ActiveCoinBalance(gctx, userID, now)
		balance = mongoBalance(b)
		return err
	})
	g.Go(func() error {
		st, err := s.Store.FindActiveStreak(gctx, userID)
		streak = st
		return err
	})
	g.Go(func() error {
		t, err := s.Store.FindRewardTransactionsByUser(gctx, userID, true, nil)
		txns = t
		return err
	})
	g.Go(func() error {
		o, err := s.PG.NonVoidOrdersByUser(gctx, userID)
		nonVoid = o
		return err
	})
	g.Go(func() error {
		uc, err := s.PG.UserCaseByUserID(gctx, userID)
		if err != nil {
			return err
		}
		if uc != nil {
			userCase = &userCaseRef{CaseID: uc.CaseID, Gender: uc.Gender, Phone: uc.PhoneNumber}
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	out := &StreakAndRewardBalance{
		RewardBalance:                    int(math.Round(balance.BalanceCoins)),
		IsUserEligibleForNewBahFlow:      true, // checkUserEligibleForNewBahFlow is hardcoded true
		BahTitle:                         BahTitle,
		PopupText:                        PopupText,
		CoinDiscountCap:                  CoinDiscountCap{Value: CoinDiscountCapValue, Type: CoinDiscountCapType},
		CoinConversionRatio:              CoinConversionRatio,
		CoinNotApplied:                   CoinNotApplied,
		CoinApplied:                      CoinApplied,
		AutoApplyCoins:                   AutoApplyCoins,
		BahbannerNewTitle:                BahBannerNewTitle,
		StreakRewardMessage:              "",   // isEligibleForNewModal is always true
		HasUserSeenBahUpdatedModalResult: true, // literal true in handler.js (inventory §7.6)
		ReminderOnBahPage:                ReminderOnBahPage,
		BahBanner:                        map[string]any{},
	}
	// threeDaysStreakCount/sevenDaysStreakCount/twentyOneDaysStreakCount stay 0: handler.js reads
	// record.days, which lives on the populated streak master, so they never increment (inventory §7.5).

	var lastLogPtr, firstLogPtr *time.Time
	if streak != nil && !streak.ID.IsZero() {
		out.CurrentDaysStreakCount = streak.StreakAchieveDays
		out.LongestDaysStreakCount = streak.LongestStreakDays
		out.LastLogDate = streak.LastDateOfLog
		out.FirstLogDate = streak.FirstDateOfLog
		if !streak.LastDateOfLog.IsZero() {
			l := streak.LastDateOfLog
			lastLogPtr = &l
		}
		if !streak.FirstDateOfLog.IsZero() {
			f := streak.FirstDateOfLog
			firstLogPtr = &f
		}
	}
	isBahLocked := !out.IsUserEligibleForNewBahFlow
	hasLoggedEver := firstLogPtr != nil || len(txns) > 0
	loggedToday := IsLoggedToday(lastLogPtr, now)

	caseID, gender := "", "M"
	if userCase != nil {
		caseID = userCase.CaseID
		gender = GenderOf(userCase.Gender)
	}
	isO8Plus := CheckO8PlusEligibility(nonVoid, now)
	isEligibleForStreakRestartBonus := CheckStreakRestartBonusEligibility(caseID, gender)

	if out.IsUserEligibleForNewBahFlow {
		coins := 100
		switch {
		case out.CurrentDaysStreakCount >= 7:
			coins = 2000
			if isO8Plus {
				coins = 2500
			}
		case out.CurrentDaysStreakCount >= 3:
			coins = 400
			if isO8Plus {
				coins = 600
			}
		default:
			if isO8Plus {
				coins = 200
			}
		}
		out.BahBanner = BahBanner{Title: BahTitle, SubTitle: "Take a step closer to healthy hair",
			CtaText: "Log Your Routine", Coins: coins, UnlockText: "Unlocking Soon!"}
	}

	if version >= VersionGateBahV3 {
		out.BannerWidgetData = GetBannerWidgetData(BannerInput{
			RewardBalance: out.RewardBalance, HasLoggedEver: hasLoggedEver, IsBahLocked: isBahLocked,
			LoggedToday: loggedToday, CurrentDaysStreakCount: out.CurrentDaysStreakCount, LastLogDate: lastLogPtr,
			IsEligibleForStreakRestartBonus: isEligibleForStreakRestartBonus, Now: now, CDNBaseURL: s.Cfg.S3ImageBaseURL,
		})
		out.CurrentDaysStreakCount = StreakBrokenRecompute(out.CurrentDaysStreakCount, lastLogPtr, now)
	}

	rewards := map[int]int{3: 100, 7: 400, 21: 2000}
	if isO8Plus {
		rewards = O8PlusStreakRewards
	}
	currentMilestone := 0
	switch {
	case out.CurrentDaysStreakCount >= 21:
		currentMilestone = 21
	case out.CurrentDaysStreakCount >= 7:
		currentMilestone = 7
	case out.CurrentDaysStreakCount >= 3:
		currentMilestone = 3
	}
	currentCoins := 100
	if isO8Plus {
		currentCoins = 200
	}
	if currentMilestone != 0 {
		currentCoins = rewards[currentMilestone]
	}

	var communityShareButton *ModalButton
	if (currentMilestone == 7 || currentMilestone == 21) && authToken != "" && s.Cfg.CommunityBaseURL != "" {
		communityShareButton = &ModalButton{Label: "Share on Community", Action: "web_page", Variant: "primary",
			URL: CommunityShareURL(s.Cfg.CommunityBaseURL, authToken, currentMilestone, currentCoins)}
	}

	var feedbackShareButton *ModalButton
	isFifteenDayEligible := false
	scopeOK := authToken != "" && gender == "F" && len(nonVoid) == 1 && out.LongestDaysStreakCount >= 3
	if scopeOK {
		details := orders.GetAllDetailsRelatedToNonVoidOrders(nonVoid, now)
		if details.RunningWeekinMonthForHairKit == 3 {
			elig, err := s.CheckFifteenDayCheckinCallEligibility(ctx, userID)
			if err != nil {
				s.Log.Warn("15-day checkin eligibility failed", "userId", userID, "error", err.Error())
			} else if elig.Eligible() {
				isFifteenDayEligible = true
				if url := s.BuildFifteenDayCheckinFeedbackURL(ctx, caseID, userID, gender, authToken); url != "" {
					feedbackShareButton = &ModalButton{Label: "Share treatment feedback", Action: "web_page", Variant: "primary", URL: url}
				}
			}
		}
	}

	sameDay := lastLogPtr != nil && firstLogPtr != nil &&
		common.UTCMidnight(*firstLogPtr).Equal(common.UTCMidnight(*lastLogPtr))
	isStreakBrokenBeforeLog := (out.CurrentDaysStreakCount == 0 && hasLoggedEver) ||
		(out.CurrentDaysStreakCount == 1 && loggedToday && out.LongestDaysStreakCount > 1 && sameDay)
	streakAfterLogging := out.CurrentDaysStreakCount
	if !loggedToday {
		streakAfterLogging++
	}
	showStreakRestartBonus := isEligibleForStreakRestartBonus && isStreakBrokenBeforeLog

	out.Modals = BuildModals(ModalsInput{
		PostLog:          GetPostLoggingModalContent(streakAfterLogging, hasLoggedEver, false, showStreakRestartBonus),
		PostLogYesterday: GetPostLoggingModalContent(streakAfterLogging, hasLoggedEver, true, showStreakRestartBonus),
		CurrentCoins:     currentCoins, CurrentMilestone: currentMilestone,
		IsStreakBrokenBeforeLog: isStreakBrokenBeforeLog, IsFifteenDayCheckinEligible: isFifteenDayEligible,
		FeedbackShareButton: feedbackShareButton, CommunityShareButton: communityShareButton,
		ShowStreakRestartModal: isEligibleForStreakRestartBonus && out.CurrentDaysStreakCount == 0 && !loggedToday,
	})

	showMissed, err := s.HasUserSeenBahUpdatedModal(ctx, userID, hasLoggedEver, lastLogPtr)
	if err != nil {
		s.Log.Warn("bah updated modal check failed", "userId", userID, "error", err.Error())
	}
	out.ShowBAHLogMissedToUser = showMissed
	return out, nil
}

type mongoBalance struct {
	BalanceCoins   float64
	EarliestExpiry *time.Time
}

type userCaseRef struct{ CaseID, Gender, Phone string }
