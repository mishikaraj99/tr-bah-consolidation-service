package legacy

import "fmt"

// ModalButton is one modal CTA.
type ModalButton struct {
	Label   string `json:"label"`
	Action  string `json:"action"`
	Variant string `json:"variant"`
	URL     string `json:"url,omitempty"`
}

// RewardsModalDescription is the rewardsModal.description object.
type RewardsModalDescription struct {
	FirstLog       string `json:"firstLog"`
	StreakComplete string `json:"streakComplete"`
}

// LogDoneModal is modals.logDoneModal.
type LogDoneModal struct {
	Type                    string        `json:"type"`
	ShowCoinsAndStreaksInfo bool          `json:"showCoinsAndStreaksInfo"`
	MediaURL                string        `json:"mediaUrl"`
	Title                   string        `json:"title"`
	TitleYesterday          string        `json:"titleYesterday"`
	Description             string        `json:"description"`
	DescriptionYesterday    string        `json:"descriptionYesterday"`
	Buttons                 []ModalButton `json:"buttons"`
	ButtonsYesterday        []ModalButton `json:"buttonsYesterday"`
}

// RewardsModal is modals.rewardsModal.
type RewardsModal struct {
	Type                    string                  `json:"type"`
	ShowCoinsAndStreaksInfo bool                    `json:"showCoinsAndStreaksInfo"`
	ConfettiAnimation       string                  `json:"confettiAnimation"`
	MediaURL                string                  `json:"mediaUrl"`
	RewardAmount            string                  `json:"rewardAmount"`
	Title                   string                  `json:"title"`
	TitleYesterday          string                  `json:"titleYesterday"`
	Description             RewardsModalDescription `json:"description"`
	Buttons                 []ModalButton           `json:"buttons"`
	ButtonsYesterday        []ModalButton           `json:"buttonsYesterday"`
}

// SimpleModal covers the fixed-copy modals (error, streakBroke, featureUpdate, newOrder, missedLog, restart bonus).
type SimpleModal struct {
	Type                    string        `json:"type"`
	ShowCoinsAndStreaksInfo bool          `json:"showCoinsAndStreaksInfo"`
	MediaURL                string        `json:"mediaUrl,omitempty"`
	Title                   string        `json:"title"`
	Description             string        `json:"description,omitempty"`
	DescriptionSecondary    string        `json:"descriptionSecondary,omitempty"`
	RetryText               string        `json:"retryText,omitempty"`
	Buttons                 []ModalButton `json:"buttons"`
}

// Modals is the modals payload of /streakAndRewardBalance.
type Modals struct {
	LogDoneModal            LogDoneModal `json:"logDoneModal"`
	RewardsModal            RewardsModal `json:"rewardsModal"`
	StreakRestartBonusModal *SimpleModal `json:"streakRestartBonusModal"`
	ErrorModal              SimpleModal  `json:"errorModal"`
	StreakBrokeModal        SimpleModal  `json:"streakBrokeModal"`
	FeatureUpdateModal      SimpleModal  `json:"featureUpdateModal"`
	NewOrderModal           SimpleModal  `json:"newOrderModal"`
	MissedLogModal          SimpleModal  `json:"missedLogModal"`
}

// ModalsInput is BuildModals' argument set.
type ModalsInput struct {
	PostLog                     PostLogContent
	PostLogYesterday            PostLogContent
	CurrentCoins                int
	CurrentMilestone            int // 0 when none
	IsStreakBrokenBeforeLog     bool
	IsFifteenDayCheckinEligible bool
	FeedbackShareButton         *ModalButton
	CommunityShareButton        *ModalButton
	ShowStreakRestartModal      bool
}

// BuildModals ports the modals block of handler.js getStreakAndRewardBalance.
func BuildModals(in ModalsInput) Modals {
	primary := func(p PostLogContent) []ModalButton {
		return []ModalButton{{Label: p.Cta, Action: p.CtaAction, Variant: "primary"}}
	}
	m := Modals{
		LogDoneModal: LogDoneModal{
			Type: "logDone", ShowCoinsAndStreaksInfo: true, MediaURL: MediaTick,
			Title: in.PostLog.Title, TitleYesterday: in.PostLogYesterday.Title,
			Description: in.PostLog.Description, DescriptionYesterday: in.PostLogYesterday.Description,
			Buttons: primary(in.PostLog), ButtonsYesterday: primary(in.PostLogYesterday),
		},
		ErrorModal: SimpleModal{Type: "logFailed", ShowCoinsAndStreaksInfo: true, MediaURL: MediaError,
			Title: "Log Failed!", Description: "Something went wrong.", RetryText: "Please retry.",
			Buttons: []ModalButton{{Label: "Try Again", Action: "close", Variant: "primary"}}},
		StreakBrokeModal: SimpleModal{Type: "streakBroke", ShowCoinsAndStreaksInfo: true, MediaURL: MediaStreakBroke,
			Title: "Your streak broke ☹️", Description: "Log for 3 days to earn 100 coins.",
			DescriptionSecondary: "If you miss for 2 days straight, logging resets. Don't worry, total coins earned stay with you.",
			Buttons: []ModalButton{
				{Label: "Log for Yesterday", Action: "setDateToYesterday", Variant: "secondary"},
				{Label: "Log for Today", Action: "setDateToToday", Variant: "primary"}}},
		FeatureUpdateModal: SimpleModal{Type: "featureUpdate", ShowCoinsAndStreaksInfo: false, MediaURL: MediaRocket,
			Title: "New features.\nJust for you.", Description: "Designed to make things easier,\nfaster, and better. Take a look!",
			Buttons: []ModalButton{{Label: "Okay", Action: "close", Variant: "primary"}}},
		NewOrderModal: SimpleModal{Type: "orderDelivered", ShowCoinsAndStreaksInfo: false, MediaURL: MediaKitContainer,
			Title: "Congrats! Your order is delivered.", Description: "You will see new products\nfrom your delivery date",
			Buttons: []ModalButton{{Label: "Okay", Action: "close", Variant: "primary"}}},
		MissedLogModal: SimpleModal{Type: "logMissed", ShowCoinsAndStreaksInfo: false, MediaURL: MediaMissed,
			Title: "Missed Logging Yesterday", Description: "Last chance to log for yesterday.",
			Buttons: []ModalButton{
				{Label: "Did Not Use Kit Yesterday", Action: "markMissed", Variant: "danger"},
				{Label: "Log for Yesterday", Action: "setDateToYesterday", Variant: "primary"}}},
	}

	firstLog := "You won rewards for doing your first log"
	if in.IsStreakBrokenBeforeLog {
		firstLog = in.PostLog.Title
	}
	milestone := in.CurrentMilestone
	if milestone == 0 {
		milestone = 3
	}
	streakComplete := fmt.Sprintf("You won rewards for completing\n%d-Day streak.", milestone)
	if in.IsFifteenDayCheckinEligible {
		streakComplete = FifteenDayCheckinSubtext
	}
	rewardButtons := primary(in.PostLog)
	if in.FeedbackShareButton != nil {
		rewardButtons = []ModalButton{*in.FeedbackShareButton}
	}
	rewardButtonsYesterday := primary(in.PostLogYesterday)
	if in.CommunityShareButton != nil {
		rewardButtons = append(rewardButtons, *in.CommunityShareButton)
		rewardButtonsYesterday = append(rewardButtonsYesterday, *in.CommunityShareButton)
	}
	m.RewardsModal = RewardsModal{
		Type: "streakModal", ShowCoinsAndStreaksInfo: true, ConfettiAnimation: MediaConfetti, MediaURL: MediaYellowCoin,
		RewardAmount: fmt.Sprintf("%d", in.CurrentCoins),
		Title:        in.PostLog.Title, TitleYesterday: in.PostLogYesterday.Title,
		Description: RewardsModalDescription{FirstLog: firstLog, StreakComplete: streakComplete},
		Buttons:     rewardButtons, ButtonsYesterday: rewardButtonsYesterday,
	}
	if in.ShowStreakRestartModal {
		m.StreakRestartBonusModal = &SimpleModal{Type: "streakRestartBonus", ShowCoinsAndStreaksInfo: true,
			Title:   fmt.Sprintf("Bonus %d coins credited!", StreakRestartBonusCoins),
			Buttons: []ModalButton{{Label: "Okay", Action: "close", Variant: "primary"}}}
	}
	return m
}

// CommunityShareURL builds the 7/21-day community share link.
func CommunityShareURL(base, authToken string, milestone, coins int) string {
	return fmt.Sprintf("%slanding/%s?share=true&streak_count=%d&reward_coins=%d&cross=no&preventBack=true", base, authToken, milestone, coins)
}
