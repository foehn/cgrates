/*
Real-time Charging System for Telecom & ISP environments
Copyright (C) ITsysCOM GmbH

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>
*/

package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cgrates/cgrates/cache2go"
	"github.com/cgrates/cgrates/config"
	"github.com/cgrates/cgrates/engine"
	"github.com/cgrates/cgrates/scheduler"
	"github.com/cgrates/cgrates/utils"
)

const (
	OK = utils.OK
)

type ApierV1 struct {
	StorDb      engine.LoadStorage
	RatingDb    engine.RatingStorage
	AccountDb   engine.AccountingStorage
	CdrDb       engine.CdrStorage
	LogDb       engine.LogStorage
	Sched       *scheduler.Scheduler
	Config      *config.CGRConfig
	Responder   *engine.Responder
	CdrStatsSrv engine.StatsInterface
	Users       engine.UserService
}

func (self *ApierV1) GetDestination(dstId string, reply *engine.Destination) error {
	if dst, err := self.RatingDb.GetDestination(dstId); err != nil {
		return utils.ErrNotFound
	} else {
		*reply = *dst
	}
	return nil
}

func (apier *ApierV1) GetSharedGroup(sgId string, reply *engine.SharedGroup) error {
	if sg, err := apier.RatingDb.GetSharedGroup(sgId, false); err != nil && err != utils.ErrNotFound { // Not found is not an error here
		return err
	} else {
		if sg != nil {
			*reply = *sg
		}
	}
	return nil
}

func (self *ApierV1) SetDestination(attrs utils.AttrSetDestination, reply *string) error {
	if missing := utils.MissingStructFields(&attrs, []string{"Id", "Prefixes"}); len(missing) != 0 {
		return utils.NewErrMandatoryIeMissing(missing...)
	}

	if !attrs.Overwrite {
		if exists, err := self.RatingDb.HasData(utils.DESTINATION_PREFIX, attrs.Id); err != nil {
			return utils.NewErrServerError(err)
		} else if exists {
			return utils.ErrExists
		}
	}
	dest := &engine.Destination{Id: attrs.Id, Prefixes: attrs.Prefixes}
	if err := self.RatingDb.SetDestination(dest); err != nil {
		return utils.NewErrServerError(err)
	}
	self.RatingDb.CacheRatingPrefixValues(map[string][]string{utils.DESTINATION_PREFIX: []string{dest.Id}})
	*reply = OK
	return nil
}

func (self *ApierV1) GetRatingPlan(rplnId string, reply *engine.RatingPlan) error {
	if rpln, err := self.RatingDb.GetRatingPlan(rplnId, false); err != nil {
		return utils.ErrNotFound
	} else {
		*reply = *rpln
	}
	return nil
}

// Get balance
func (self *ApierV1) GetAccount(attr *utils.AttrGetAccount, reply *engine.Account) error {
	tag := fmt.Sprintf("%s:%s:%s", attr.Direction, attr.Tenant, attr.Account)
	userBalance, err := self.AccountDb.GetAccount(tag)
	if err != nil {
		return err
	}

	*reply = *userBalance
	return nil
}

type AttrAddBalance struct {
	Tenant        string
	Account       string
	BalanceUuid   string
	BalanceId     string
	BalanceType   string
	Direction     string
	Value         float64
	ExpiryTime    string
	RatingSubject string
	DestinationId string
	Weight        float64
	SharedGroup   string
	Overwrite     bool // When true it will reset if the balance is already there
	Disabled      bool
}

func (self *ApierV1) AddBalance(attr *AttrAddBalance, reply *string) error {
	expTime, err := utils.ParseDate(attr.ExpiryTime)
	if err != nil {
		*reply = err.Error()
		return err
	}
	tag := utils.ConcatenatedKey(attr.Direction, attr.Tenant, attr.Account)
	if _, err := self.AccountDb.GetAccount(tag); err != nil {
		// create user balance if not exists
		account := &engine.Account{
			Id: tag,
		}
		if err := self.AccountDb.SetAccount(account); err != nil {
			*reply = err.Error()
			return err
		}
	}
	at := &engine.ActionPlan{
		AccountIds: []string{tag},
	}
	if attr.Direction == "" {
		attr.Direction = utils.OUT
	}
	aType := engine.DEBIT
	// reverse the sign as it is a debit
	attr.Value = -attr.Value

	if attr.Overwrite {
		aType = engine.DEBIT_RESET
	}
	at.SetActions(engine.Actions{
		&engine.Action{
			ActionType:  aType,
			BalanceType: attr.BalanceType,
			Direction:   attr.Direction,
			Balance: &engine.Balance{
				Uuid:           attr.BalanceUuid,
				Id:             attr.BalanceId,
				Value:          attr.Value,
				ExpirationDate: expTime,
				RatingSubject:  attr.RatingSubject,
				DestinationIds: attr.DestinationId,
				Weight:         attr.Weight,
				SharedGroup:    attr.SharedGroup,
				Disabled:       attr.Disabled,
			},
		},
	})
	if err := at.Execute(); err != nil {
		*reply = err.Error()
		return err
	}
	*reply = OK
	return nil
}

func (self *ApierV1) EnableDisableBalance(attr *AttrAddBalance, reply *string) error {
	expTime, err := utils.ParseDate(attr.ExpiryTime)
	if err != nil {
		*reply = err.Error()
		return err
	}
	tag := utils.ConcatenatedKey(attr.Direction, attr.Tenant, attr.Account)
	if _, err := self.AccountDb.GetAccount(tag); err != nil {
		return utils.ErrNotFound
	}
	at := &engine.ActionPlan{
		AccountIds: []string{tag},
	}
	if attr.Direction == "" {
		attr.Direction = utils.OUT
	}
	at.SetActions(engine.Actions{
		&engine.Action{
			ActionType:  engine.ENABLE_DISABLE_BALANCE,
			BalanceType: attr.BalanceType,
			Direction:   attr.Direction,
			Balance: &engine.Balance{
				Id:             attr.BalanceId,
				Value:          attr.Value,
				ExpirationDate: expTime,
				RatingSubject:  attr.RatingSubject,
				DestinationIds: attr.DestinationId,
				Weight:         attr.Weight,
				SharedGroup:    attr.SharedGroup,
				Disabled:       attr.Disabled,
			},
		},
	})
	if err := at.Execute(); err != nil {
		*reply = err.Error()
		return err
	}
	*reply = OK
	return nil
}

func (self *ApierV1) ExecuteAction(attr *utils.AttrExecuteAction, reply *string) error {
	tag := fmt.Sprintf("%s:%s:%s", attr.Direction, attr.Tenant, attr.Account)
	at := &engine.ActionPlan{
		AccountIds: []string{tag},
		ActionsId:  attr.ActionsId,
	}
	if err := at.Execute(); err != nil {
		*reply = err.Error()
		return err
	}
	*reply = OK
	return nil
}

type AttrLoadDestination struct {
	TPid          string
	DestinationId string
}

// Load destinations from storDb into dataDb.
func (self *ApierV1) LoadDestination(attrs AttrLoadDestination, reply *string) error {
	if len(attrs.TPid) == 0 {
		return utils.NewErrMandatoryIeMissing("TPid")
	}
	dbReader := engine.NewTpReader(self.RatingDb, self.AccountDb, self.StorDb, attrs.TPid, self.Config.DefaultTimezone, self.Config.LoadHistorySize)
	if loaded, err := dbReader.LoadDestinationsFiltered(attrs.DestinationId); err != nil {
		return utils.NewErrServerError(err)
	} else if !loaded {
		return utils.ErrNotFound
	}
	//Automatic cache of the newly inserted rating plan
	destIds := []string{utils.DESTINATION_PREFIX + attrs.DestinationId}
	if len(attrs.DestinationId) == 0 {
		destIds = nil // Cache all destinations, temporary here until we add ApierV2.LoadDestinations
	}
	if err := self.RatingDb.CacheRatingPrefixValues(map[string][]string{
		utils.DESTINATION_PREFIX: destIds}); err != nil {
		return err
	}
	*reply = OK
	return nil
}

// Load derived chargers from storDb into dataDb.
func (self *ApierV1) LoadDerivedChargers(attrs utils.TPDerivedChargers, reply *string) error {
	if len(attrs.TPid) == 0 {
		return utils.NewErrMandatoryIeMissing("TPid")
	}
	dbReader := engine.NewTpReader(self.RatingDb, self.AccountDb, self.StorDb, attrs.TPid, self.Config.DefaultTimezone, self.Config.LoadHistorySize)
	dc := engine.APItoModelDerivedCharger(&attrs)
	if err := dbReader.LoadDerivedChargersFiltered(&dc[0], true); err != nil {
		return utils.NewErrServerError(err)
	}
	//Automatic cache of the newly inserted rating plan
	var derivedChargingKeys []string
	if len(attrs.Direction) != 0 && len(attrs.Tenant) != 0 && len(attrs.Category) != 0 && len(attrs.Account) != 0 && len(attrs.Subject) != 0 {
		derivedChargingKeys = []string{utils.DERIVEDCHARGERS_PREFIX + attrs.GetDerivedChargersKey()}
	}
	if err := self.RatingDb.CacheRatingPrefixValues(map[string][]string{utils.DERIVEDCHARGERS_PREFIX: derivedChargingKeys}); err != nil {
		return err
	}
	*reply = OK
	return nil
}

type AttrLoadRatingPlan struct {
	TPid         string
	RatingPlanId string
}

// Process dependencies and load a specific rating plan from storDb into dataDb.
func (self *ApierV1) LoadRatingPlan(attrs AttrLoadRatingPlan, reply *string) error {
	if len(attrs.TPid) == 0 {
		return utils.NewErrMandatoryIeMissing("TPid")
	}
	dbReader := engine.NewTpReader(self.RatingDb, self.AccountDb, self.StorDb, attrs.TPid, self.Config.DefaultTimezone, self.Config.LoadHistorySize)
	if loaded, err := dbReader.LoadRatingPlansFiltered(attrs.RatingPlanId); err != nil {
		return utils.NewErrServerError(err)
	} else if !loaded {
		return utils.ErrNotFound
	}
	//Automatic cache of the newly inserted rating plan
	var changedRPlKeys []string
	if len(attrs.TPid) != 0 {
		if attrs.RatingPlanId != "" {
			changedRPlKeys = []string{utils.RATING_PLAN_PREFIX + attrs.RatingPlanId}
		} else {
			changedRPlKeys = nil
		}
	}
	if err := self.RatingDb.CacheRatingPrefixValues(map[string][]string{
		utils.DESTINATION_PREFIX: nil,
		utils.RATING_PLAN_PREFIX: changedRPlKeys,
	}); err != nil {
		return err
	}
	*reply = OK
	return nil
}

// Process dependencies and load a specific rating profile from storDb into dataDb.
func (self *ApierV1) LoadRatingProfile(attrs utils.TPRatingProfile, reply *string) error {
	if len(attrs.TPid) == 0 {
		return utils.NewErrMandatoryIeMissing("TPid")
	}
	dbReader := engine.NewTpReader(self.RatingDb, self.AccountDb, self.StorDb, attrs.TPid, self.Config.DefaultTimezone, self.Config.LoadHistorySize)
	rp := engine.APItoModelRatingProfile(&attrs)
	if err := dbReader.LoadRatingProfilesFiltered(&rp[0]); err != nil {
		return utils.NewErrServerError(err)
	}
	//Automatic cache of the newly inserted rating profile
	var ratingProfile []string
	if attrs.KeyId() != ":::" { // if has some filters
		ratingProfile = []string{utils.RATING_PROFILE_PREFIX + attrs.KeyId()}
	}
	if err := self.RatingDb.CacheRatingPrefixValues(map[string][]string{utils.RATING_PROFILE_PREFIX: ratingProfile}); err != nil {
		return err
	}
	*reply = OK
	return nil
}

type AttrLoadSharedGroup struct {
	TPid          string
	SharedGroupId string
}

// Load destinations from storDb into dataDb.
func (self *ApierV1) LoadSharedGroup(attrs AttrLoadSharedGroup, reply *string) error {
	if len(attrs.TPid) == 0 {
		return utils.NewErrMandatoryIeMissing("TPid")
	}
	dbReader := engine.NewTpReader(self.RatingDb, self.AccountDb, self.StorDb, attrs.TPid, self.Config.DefaultTimezone, self.Config.LoadHistorySize)
	if err := dbReader.LoadSharedGroupsFiltered(attrs.SharedGroupId, true); err != nil {
		return utils.NewErrServerError(err)
	}
	//Automatic cache of the newly inserted rating plan
	var changedSharedGroup []string
	if len(attrs.SharedGroupId) != 0 {
		changedSharedGroup = []string{utils.SHARED_GROUP_PREFIX + attrs.SharedGroupId}
	}
	if err := self.RatingDb.CacheRatingPrefixValues(map[string][]string{utils.SHARED_GROUP_PREFIX: changedSharedGroup}); err != nil {
		return err
	}
	*reply = OK
	return nil
}

type AttrLoadCdrStats struct {
	TPid       string
	CdrStatsId string
}

// Load destinations from storDb into dataDb.
func (self *ApierV1) LoadCdrStats(attrs AttrLoadCdrStats, reply *string) error {
	if len(attrs.TPid) == 0 {
		return utils.NewErrMandatoryIeMissing("TPid")
	}
	dbReader := engine.NewTpReader(self.RatingDb, self.AccountDb, self.StorDb, attrs.TPid, self.Config.DefaultTimezone, self.Config.LoadHistorySize)
	if err := dbReader.LoadCdrStatsFiltered(attrs.CdrStatsId, true); err != nil {
		return utils.NewErrServerError(err)
	}
	*reply = OK
	return nil
}

type AttrLoadTpFromStorDb struct {
	TPid     string
	FlushDb  bool // Flush ratingDb before loading
	DryRun   bool // Only simulate, no write
	Validate bool // Run structural checks
}

// Loads complete data in a TP from storDb
func (self *ApierV1) LoadTariffPlanFromStorDb(attrs AttrLoadTpFromStorDb, reply *string) error {
	if len(attrs.TPid) == 0 {
		return utils.NewErrMandatoryIeMissing("TPid")
	}
	dbReader := engine.NewTpReader(self.RatingDb, self.AccountDb, self.StorDb, attrs.TPid, self.Config.DefaultTimezone, self.Config.LoadHistorySize)
	if err := dbReader.LoadAll(); err != nil {
		return utils.NewErrServerError(err)
	}
	if attrs.Validate {
		if !dbReader.IsValid() {
			*reply = OK
			return errors.New("invalid data")
		}
	}
	if attrs.DryRun {
		*reply = OK
		return nil // Mission complete, no errors
	}
	if err := dbReader.WriteToDatabase(attrs.FlushDb, false); err != nil {
		return utils.NewErrServerError(err)
	}
	// Make sure the items are in the cache
	dstIds, _ := dbReader.GetLoadedIds(utils.DESTINATION_PREFIX)
	dstKeys := make([]string, len(dstIds))
	for idx, dId := range dstIds {
		dstKeys[idx] = utils.DESTINATION_PREFIX + dId // Cache expects them as redis keys
	}
	rplIds, _ := dbReader.GetLoadedIds(utils.RATING_PLAN_PREFIX)
	rpKeys := make([]string, len(rplIds))
	for idx, rpId := range rplIds {
		rpKeys[idx] = utils.RATING_PLAN_PREFIX + rpId
	}
	rpfIds, _ := dbReader.GetLoadedIds(utils.RATING_PROFILE_PREFIX)
	rpfKeys := make([]string, len(rpfIds))
	for idx, rpfId := range rpfIds {
		rpfKeys[idx] = utils.RATING_PROFILE_PREFIX + rpfId
	}
	actIds, _ := dbReader.GetLoadedIds(utils.ACTION_PREFIX)
	actKeys := make([]string, len(actIds))
	for idx, actId := range actIds {
		actKeys[idx] = utils.ACTION_PREFIX + actId
	}
	shgIds, _ := dbReader.GetLoadedIds(utils.SHARED_GROUP_PREFIX)
	shgKeys := make([]string, len(shgIds))
	for idx, shgId := range shgIds {
		shgKeys[idx] = utils.SHARED_GROUP_PREFIX + shgId
	}
	aliases, _ := dbReader.GetLoadedIds(utils.ALIASES_PREFIX)
	alsKeys := make([]string, len(aliases))
	for idx, alias := range aliases {
		alsKeys[idx] = utils.ALIASES_PREFIX + alias
	}
	lcrIds, _ := dbReader.GetLoadedIds(utils.LCR_PREFIX)
	lcrKeys := make([]string, len(lcrIds))
	for idx, lcrId := range lcrIds {
		lcrKeys[idx] = utils.LCR_PREFIX + lcrId
	}
	dcs, _ := dbReader.GetLoadedIds(utils.DERIVEDCHARGERS_PREFIX)
	dcsKeys := make([]string, len(dcs))
	for idx, dc := range dcs {
		dcsKeys[idx] = utils.DERIVEDCHARGERS_PREFIX + dc
	}
	utils.Logger.Info("ApierV1.LoadTariffPlanFromStorDb, reloading cache.")
	if err := self.RatingDb.CacheRatingPrefixValues(map[string][]string{
		utils.DESTINATION_PREFIX:     dstKeys,
		utils.RATING_PLAN_PREFIX:     rpKeys,
		utils.RATING_PROFILE_PREFIX:  rpfKeys,
		utils.LCR_PREFIX:             lcrKeys,
		utils.DERIVEDCHARGERS_PREFIX: dcsKeys,
		utils.ACTION_PREFIX:          actKeys,
		utils.SHARED_GROUP_PREFIX:    shgKeys,
	}); err != nil {
		return err
	}
	if err := self.AccountDb.CacheAccountingPrefixValues(map[string][]string{
		utils.ALIASES_PREFIX: alsKeys,
	}); err != nil {
		return err
	}
	aps, _ := dbReader.GetLoadedIds(utils.ACTION_TIMING_PREFIX)
	if len(aps) != 0 && self.Sched != nil {
		utils.Logger.Info("ApierV1.LoadTariffPlanFromStorDb, reloading scheduler.")
		self.Sched.LoadActionPlans(self.RatingDb)
		self.Sched.Restart()
	}
	cstKeys, _ := dbReader.GetLoadedIds(utils.CDR_STATS_PREFIX)
	if len(cstKeys) != 0 && self.CdrStatsSrv != nil {
		if err := self.CdrStatsSrv.ReloadQueues(cstKeys, nil); err != nil {
			return err
		}
	}
	userKeys, _ := dbReader.GetLoadedIds(utils.USERS_PREFIX)
	if len(userKeys) != 0 && self.Users != nil {
		var r string
		if err := self.Users.ReloadUsers("", &r); err != nil {
			return err
		}
	}
	*reply = OK
	return nil
}

func (self *ApierV1) ImportTariffPlanFromFolder(attrs utils.AttrImportTPFromFolder, reply *string) error {
	if missing := utils.MissingStructFields(&attrs, []string{"TPid", "FolderPath"}); len(missing) != 0 {
		return utils.NewErrMandatoryIeMissing(missing...)
	}
	if len(attrs.CsvSeparator) == 0 {
		attrs.CsvSeparator = ","
	}
	if fi, err := os.Stat(attrs.FolderPath); err != nil {
		if strings.HasSuffix(err.Error(), "no such file or directory") {
			return utils.ErrInvalidPath
		}
		return utils.NewErrServerError(err)
	} else if !fi.IsDir() {
		return utils.ErrInvalidPath
	}
	csvImporter := engine.TPCSVImporter{
		TPid:     attrs.TPid,
		StorDb:   self.StorDb,
		DirPath:  attrs.FolderPath,
		Sep:      rune(attrs.CsvSeparator[0]),
		Verbose:  false,
		ImportId: attrs.RunId,
	}
	if err := csvImporter.Run(); err != nil {
		return utils.NewErrServerError(err)
	}
	*reply = utils.OK
	return nil
}

type AttrSetRatingProfile struct {
	Tenant                string                      // Tenant's Id
	Category              string                      // TypeOfRecord
	Direction             string                      // Traffic direction, OUT is the only one supported for now
	Subject               string                      // Rating subject, usually the same as account
	Overwrite             bool                        // Overwrite if exists
	RatingPlanActivations []*utils.TPRatingActivation // Activate rating plans at specific time
}

// Sets a specific rating profile working with data directly in the RatingDb without involving storDb
func (self *ApierV1) SetRatingProfile(attrs AttrSetRatingProfile, reply *string) error {
	if missing := utils.MissingStructFields(&attrs, []string{"Tenant", "TOR", "Direction", "Subject", "RatingPlanActivations"}); len(missing) != 0 {
		return utils.NewErrMandatoryIeMissing(missing...)
	}
	for _, rpa := range attrs.RatingPlanActivations {
		if missing := utils.MissingStructFields(rpa, []string{"ActivationTime", "RatingPlanId"}); len(missing) != 0 {
			return fmt.Errorf("%s:RatingPlanActivation:%v", utils.ErrMandatoryIeMissing.Error(), missing)
		}
	}
	tpRpf := utils.TPRatingProfile{Tenant: attrs.Tenant, Category: attrs.Category, Direction: attrs.Direction, Subject: attrs.Subject}
	keyId := tpRpf.KeyId()
	if !attrs.Overwrite {
		if exists, err := self.RatingDb.HasData(utils.RATING_PROFILE_PREFIX, keyId); err != nil {
			return utils.NewErrServerError(err)
		} else if exists {
			return utils.ErrExists
		}
	}
	rpfl := &engine.RatingProfile{Id: keyId, RatingPlanActivations: make(engine.RatingPlanActivations, len(attrs.RatingPlanActivations))}
	for idx, ra := range attrs.RatingPlanActivations {
		at, err := utils.ParseDate(ra.ActivationTime)
		if err != nil {
			return fmt.Errorf(fmt.Sprintf("%s:Cannot parse activation time from %v", utils.ErrServerError.Error(), ra.ActivationTime))
		}
		if exists, err := self.RatingDb.HasData(utils.RATING_PLAN_PREFIX, ra.RatingPlanId); err != nil {
			return utils.NewErrServerError(err)
		} else if !exists {
			return fmt.Errorf(fmt.Sprintf("%s:RatingPlanId:%s", utils.ErrNotFound.Error(), ra.RatingPlanId))
		}
		rpfl.RatingPlanActivations[idx] = &engine.RatingPlanActivation{ActivationTime: at, RatingPlanId: ra.RatingPlanId,
			FallbackKeys: utils.FallbackSubjKeys(tpRpf.Direction, tpRpf.Tenant, tpRpf.Category, ra.FallbackSubjects)}
	}
	if err := self.RatingDb.SetRatingProfile(rpfl); err != nil {
		return utils.NewErrServerError(err)
	}
	//Automatic cache of the newly inserted rating profile
	if err := self.RatingDb.CacheRatingPrefixValues(map[string][]string{
		utils.RATING_PROFILE_PREFIX: []string{utils.RATING_PROFILE_PREFIX + keyId},
	}); err != nil {
		return err
	}
	*reply = OK
	return nil
}

func (self *ApierV1) SetActions(attrs utils.AttrSetActions, reply *string) error {
	if missing := utils.MissingStructFields(&attrs, []string{"ActionsId", "Actions"}); len(missing) != 0 {
		return utils.NewErrMandatoryIeMissing(missing...)
	}
	for _, action := range attrs.Actions {
		requiredFields := []string{"Identifier", "Weight"}
		if action.BalanceType != "" { // Add some inter-dependent parameters - if balanceType then we are not talking about simply calling actions
			requiredFields = append(requiredFields, "Direction", "Units")
		}
		if missing := utils.MissingStructFields(action, requiredFields); len(missing) != 0 {
			return fmt.Errorf("%s:Action:%s:%v", utils.ErrMandatoryIeMissing.Error(), action.Identifier, missing)
		}
	}
	if !attrs.Overwrite {
		if exists, err := self.RatingDb.HasData(utils.ACTION_PREFIX, attrs.ActionsId); err != nil {
			return utils.NewErrServerError(err)
		} else if exists {
			return utils.ErrExists
		}
	}
	storeActions := make(engine.Actions, len(attrs.Actions))
	for idx, apiAct := range attrs.Actions {
		a := &engine.Action{
			Id:               utils.GenUUID(),
			ActionType:       apiAct.Identifier,
			BalanceType:      apiAct.BalanceType,
			Direction:        apiAct.Direction,
			Weight:           apiAct.Weight,
			ExpirationString: apiAct.ExpiryTime,
			ExtraParameters:  apiAct.ExtraParameters,
			Balance: &engine.Balance{
				Uuid:           utils.GenUUID(),
				Id:             apiAct.BalanceId,
				Value:          apiAct.Units,
				Weight:         apiAct.BalanceWeight,
				DestinationIds: apiAct.DestinationIds,
				RatingSubject:  apiAct.RatingSubject,
				SharedGroup:    apiAct.SharedGroup,
			},
		}
		storeActions[idx] = a
	}
	if err := self.RatingDb.SetActions(attrs.ActionsId, storeActions); err != nil {
		return utils.NewErrServerError(err)
	}
	self.RatingDb.CacheRatingPrefixes(utils.ACTION_PREFIX)
	*reply = OK
	return nil
}

// Retrieves actions attached to specific ActionsId within cache
func (self *ApierV1) GetActions(actsId string, reply *[]*utils.TPAction) error {
	if len(actsId) == 0 {
		return fmt.Errorf("%s ActionsId: %s", utils.ErrMandatoryIeMissing.Error(), actsId)
	}
	acts := make([]*utils.TPAction, 0)
	engActs, err := self.RatingDb.GetActions(actsId, false)
	if err != nil {
		return utils.NewErrServerError(err)
	}
	for _, engAct := range engActs {
		act := &utils.TPAction{Identifier: engAct.ActionType,
			BalanceType:     engAct.BalanceType,
			Direction:       engAct.Direction,
			ExpiryTime:      engAct.ExpirationString,
			ExtraParameters: engAct.ExtraParameters,
			Weight:          engAct.Weight,
		}
		if engAct.Balance != nil {
			act.Units = engAct.Balance.GetValue()
			act.DestinationIds = engAct.Balance.DestinationIds
			act.RatingSubject = engAct.Balance.RatingSubject
			act.SharedGroup = engAct.Balance.SharedGroup
			act.BalanceWeight = engAct.Balance.Weight
		}
		acts = append(acts, act)
	}
	*reply = acts
	return nil
}

type AttrSetActionPlan struct {
	Id              string           // Profile id
	ActionPlan      []*ApiActionPlan // Set of actions this Actions profile will perform
	Overwrite       bool             // If previously defined, will be overwritten
	ReloadScheduler bool             // Enables automatic reload of the scheduler (eg: useful when adding a single action timing)
}

type ApiActionPlan struct {
	ActionsId string  // Actions id
	Years     string  // semicolon separated list of years this timing is valid on, *any or empty supported
	Months    string  // semicolon separated list of months this timing is valid on, *any or empty supported
	MonthDays string  // semicolon separated list of month's days this timing is valid on, *any or empty supported
	WeekDays  string  // semicolon separated list of week day names this timing is valid on *any or empty supported
	Time      string  // String representing the time this timing starts on, *asap supported
	Weight    float64 // Binding's weight
}

func (self *ApierV1) SetActionPlan(attrs AttrSetActionPlan, reply *string) error {
	if missing := utils.MissingStructFields(&attrs, []string{"Id", "ActionPlan"}); len(missing) != 0 {
		return utils.NewErrMandatoryIeMissing(missing...)
	}
	for _, at := range attrs.ActionPlan {
		requiredFields := []string{"ActionsId", "Time", "Weight"}
		if missing := utils.MissingStructFields(at, requiredFields); len(missing) != 0 {
			return fmt.Errorf("%s:Action:%s:%v", utils.ErrMandatoryIeMissing.Error(), at.ActionsId, missing)
		}
	}
	if !attrs.Overwrite {
		if exists, err := self.RatingDb.HasData(utils.ACTION_TIMING_PREFIX, attrs.Id); err != nil {
			return utils.NewErrServerError(err)
		} else if exists {
			return utils.ErrExists
		}
	}
	storeAtms := make(engine.ActionPlans, len(attrs.ActionPlan))
	for idx, apiAtm := range attrs.ActionPlan {
		if exists, err := self.RatingDb.HasData(utils.ACTION_PREFIX, apiAtm.ActionsId); err != nil {
			return utils.NewErrServerError(err)
		} else if !exists {
			return fmt.Errorf("%s:%s", utils.ErrBrokenReference.Error(), apiAtm.ActionsId)
		}
		timing := new(engine.RITiming)
		timing.Years.Parse(apiAtm.Years, ";")
		timing.Months.Parse(apiAtm.Months, ";")
		timing.MonthDays.Parse(apiAtm.MonthDays, ";")
		timing.WeekDays.Parse(apiAtm.WeekDays, ";")
		timing.StartTime = apiAtm.Time
		at := &engine.ActionPlan{
			Uuid:      utils.GenUUID(),
			Id:        attrs.Id,
			Weight:    apiAtm.Weight,
			Timing:    &engine.RateInterval{Timing: timing},
			ActionsId: apiAtm.ActionsId,
		}
		storeAtms[idx] = at
	}
	if err := self.RatingDb.SetActionPlans(attrs.Id, storeAtms); err != nil {
		return utils.NewErrServerError(err)
	}
	if attrs.ReloadScheduler {
		if self.Sched == nil {
			return errors.New("SCHEDULER_NOT_ENABLED")
		}
		self.Sched.LoadActionPlans(self.RatingDb)
		self.Sched.Restart()
	}
	*reply = OK
	return nil
}

type AttrAddActionTrigger struct {
	ActionTriggersId      string
	Tenant                string
	Account               string
	ThresholdType         string
	ThresholdValue        float64
	BalanceId             string
	BalanceType           string
	BalanceDirection      string
	BalanceDestinationIds string
	BalanceRatingSubject  string //ToDo
	BalanceWeight         float64
	BalanceExpiryTime     string
	BalanceSharedGroup    string //ToDo
	Weight                float64
	ActionsId             string
}

func (self *ApierV1) AddTriggeredAction(attr AttrAddActionTrigger, reply *string) error {
	if attr.BalanceDirection == "" {
		attr.BalanceDirection = utils.OUT
	}
	balExpiryTime, err := utils.ParseTimeDetectLayout(attr.BalanceExpiryTime, self.Config.DefaultTimezone)
	if err != nil {
		return utils.NewErrServerError(err)
	}
	at := &engine.ActionTrigger{
		Id:                    attr.ActionTriggersId,
		ThresholdType:         attr.ThresholdType,
		ThresholdValue:        attr.ThresholdValue,
		BalanceId:             attr.BalanceId,
		BalanceType:           attr.BalanceType,
		BalanceDirection:      attr.BalanceDirection,
		BalanceDestinationIds: attr.BalanceDestinationIds,
		BalanceWeight:         attr.BalanceWeight,
		BalanceExpirationDate: balExpiryTime,
		Weight:                attr.Weight,
		ActionsId:             attr.ActionsId,
		Executed:              false,
	}

	tag := utils.AccountKey(attr.Tenant, attr.Account, attr.BalanceDirection)
	_, err = engine.Guardian.Guard(func() (interface{}, error) {
		userBalance, err := self.AccountDb.GetAccount(tag)
		if err != nil {
			return 0, err
		}

		userBalance.ActionTriggers = append(userBalance.ActionTriggers, at)

		if err = self.AccountDb.SetAccount(userBalance); err != nil {
			return 0, err
		}
		return 0, nil
	}, 0, tag)
	if err != nil {
		*reply = err.Error()
		return err
	}
	*reply = OK
	return nil
}

type AttrResetTriggeredAction struct {
	Id                   string
	Tenant               string
	Account              string
	Direction            string
	BalanceType          string
	ThresholdType        string
	ThresholdValue       float64
	DestinationId        string
	BalanceWeight        float64
	BalanceRatingSubject string
	BalanceSharedGroup   string
}

func (self *ApierV1) ResetTriggeredActions(attr AttrResetTriggeredAction, reply *string) error {
	var a *engine.Action
	if attr.Id != "" {
		// we can identify the trigge by the id
		a = &engine.Action{Id: attr.Id}
	} else {
		if attr.Direction == "" {
			attr.Direction = utils.OUT
		}
		extraParameters, err := json.Marshal(struct {
			ThresholdType        string
			ThresholdValue       float64
			DestinationId        string
			BalanceWeight        float64
			BalanceRatingSubject string
			BalanceSharedGroup   string
		}{
			attr.ThresholdType,
			attr.ThresholdValue,
			attr.DestinationId,
			attr.BalanceWeight,
			attr.BalanceRatingSubject,
			attr.BalanceSharedGroup,
		})
		if err != nil {
			*reply = err.Error()
			return err
		}
		a = &engine.Action{
			BalanceType:     attr.BalanceType,
			Direction:       attr.Direction,
			ExtraParameters: string(extraParameters),
		}
	}
	accID := utils.AccountKey(attr.Tenant, attr.Account, attr.Direction)
	_, err := engine.Guardian.Guard(func() (interface{}, error) {
		acc, err := self.AccountDb.GetAccount(accID)
		if err != nil {
			return 0, err
		}

		acc.ResetActionTriggers(a)

		if err = self.AccountDb.SetAccount(acc); err != nil {
			return 0, err
		}
		return 0, nil
	}, 0, accID)
	if err != nil {
		*reply = err.Error()
		return err
	}
	*reply = OK
	return nil
}

// Process dependencies and load a specific AccountActions profile from storDb into dataDb.
func (self *ApierV1) LoadAccountActions(attrs utils.TPAccountActions, reply *string) error {
	if len(attrs.TPid) == 0 {
		return utils.NewErrMandatoryIeMissing("TPid")
	}
	dbReader := engine.NewTpReader(self.RatingDb, self.AccountDb, self.StorDb, attrs.TPid, self.Config.DefaultTimezone, self.Config.LoadHistorySize)
	if _, err := engine.Guardian.Guard(func() (interface{}, error) {
		aas := engine.APItoModelAccountAction(&attrs)
		if err := dbReader.LoadAccountActionsFiltered(aas); err != nil {
			return 0, err
		}
		return 0, nil
	}, 0, attrs.KeyId()); err != nil {
		return utils.NewErrServerError(err)
	}
	// ToDo: Get the action keys loaded by dbReader so we reload only these in cache
	// Need to do it before scheduler otherwise actions to run will be unknown
	if err := self.RatingDb.CacheRatingPrefixes(utils.DERIVEDCHARGERS_PREFIX, utils.ACTION_PREFIX, utils.SHARED_GROUP_PREFIX); err != nil {
		return err
	}
	if self.Sched != nil {
		self.Sched.LoadActionPlans(self.RatingDb)
		self.Sched.Restart()
	}
	*reply = OK
	return nil
}

func (self *ApierV1) ReloadScheduler(input string, reply *string) error {
	if self.Sched == nil {
		return utils.ErrNotFound
	}
	self.Sched.LoadActionPlans(self.RatingDb)
	self.Sched.Restart()
	*reply = OK
	return nil

}

func (self *ApierV1) ReloadCache(attrs utils.ApiReloadCache, reply *string) error {
	var dstKeys, rpKeys, rpfKeys, actKeys, shgKeys, lcrKeys, dcsKeys, alsKeys []string
	if len(attrs.DestinationIds) > 0 {
		dstKeys = make([]string, len(attrs.DestinationIds))
		for idx, dId := range attrs.DestinationIds {
			dstKeys[idx] = utils.DESTINATION_PREFIX + dId // Cache expects them as redis keys
		}
	}
	if len(attrs.RatingPlanIds) > 0 {
		rpKeys = make([]string, len(attrs.RatingPlanIds))
		for idx, rpId := range attrs.RatingPlanIds {
			rpKeys[idx] = utils.RATING_PLAN_PREFIX + rpId
		}
	}
	if len(attrs.RatingProfileIds) > 0 {
		rpfKeys = make([]string, len(attrs.RatingProfileIds))
		for idx, rpfId := range attrs.RatingProfileIds {
			rpfKeys[idx] = utils.RATING_PROFILE_PREFIX + rpfId
		}
	}
	if len(attrs.ActionIds) > 0 {
		actKeys = make([]string, len(attrs.ActionIds))
		for idx, actId := range attrs.ActionIds {
			actKeys[idx] = utils.ACTION_PREFIX + actId
		}
	}
	if len(attrs.SharedGroupIds) > 0 {
		shgKeys = make([]string, len(attrs.SharedGroupIds))
		for idx, shgId := range attrs.SharedGroupIds {
			shgKeys[idx] = utils.SHARED_GROUP_PREFIX + shgId
		}
	}
	if len(attrs.Aliases) > 0 {
		alsKeys = make([]string, len(attrs.Aliases))
		for idx, alias := range attrs.Aliases {
			alsKeys[idx] = utils.ALIASES_PREFIX + alias
		}
	}
	if len(attrs.LCRIds) > 0 {
		lcrKeys = make([]string, len(attrs.LCRIds))
		for idx, lcrId := range attrs.LCRIds {
			lcrKeys[idx] = utils.LCR_PREFIX + lcrId
		}
	}

	if len(attrs.DerivedChargers) > 0 {
		dcsKeys = make([]string, len(attrs.DerivedChargers))
		for idx, dc := range attrs.DerivedChargers {
			dcsKeys[idx] = utils.DERIVEDCHARGERS_PREFIX + dc
		}
	}
	if err := self.RatingDb.CacheRatingPrefixValues(map[string][]string{
		utils.DESTINATION_PREFIX:     dstKeys,
		utils.RATING_PLAN_PREFIX:     rpKeys,
		utils.RATING_PROFILE_PREFIX:  rpfKeys,
		utils.LCR_PREFIX:             lcrKeys,
		utils.DERIVEDCHARGERS_PREFIX: dcsKeys,
		utils.ACTION_PREFIX:          actKeys,
		utils.SHARED_GROUP_PREFIX:    shgKeys,
	}); err != nil {
		return err
	}
	if err := self.AccountDb.CacheAccountingPrefixValues(map[string][]string{
		utils.ALIASES_PREFIX: alsKeys,
	}); err != nil {
		return err
	}

	*reply = utils.OK
	return nil
}

func (self *ApierV1) GetCacheStats(attrs utils.AttrCacheStats, reply *utils.CacheStats) error {
	cs := new(utils.CacheStats)
	cs.Destinations = cache2go.CountEntries(utils.DESTINATION_PREFIX)
	cs.RatingPlans = cache2go.CountEntries(utils.RATING_PLAN_PREFIX)
	cs.RatingProfiles = cache2go.CountEntries(utils.RATING_PROFILE_PREFIX)
	cs.Actions = cache2go.CountEntries(utils.ACTION_PREFIX)
	cs.SharedGroups = cache2go.CountEntries(utils.SHARED_GROUP_PREFIX)
	cs.DerivedChargers = cache2go.CountEntries(utils.DERIVEDCHARGERS_PREFIX)
	cs.LcrProfiles = cache2go.CountEntries(utils.LCR_PREFIX)
	cs.Aliases = cache2go.CountEntries(utils.ALIASES_PREFIX)
	if self.CdrStatsSrv != nil && self.Config.CDRStatsEnabled {
		var queueIds []string
		if err := self.CdrStatsSrv.GetQueueIds(0, &queueIds); err != nil {
			return utils.NewErrServerError(err)
		}
		cs.CdrStats = len(queueIds)
	}
	if self.Config.RaterUserServer == utils.INTERNAL {
		var ups engine.UserProfiles
		if err := self.Users.GetUsers(engine.UserProfile{}, &ups); err != nil {
			return utils.NewErrServerError(err)
		}
		cs.Users = len(ups)
	}
	if loadHistInsts, err := self.AccountDb.GetLoadHistory(1, false); err != nil || len(loadHistInsts) == 0 {
		if err != nil { // Not really an error here since we only count in cache
			utils.Logger.Err(fmt.Sprintf("ApierV1.GetCacheStats, error on GetLoadHistory: %s"))
		}
		cs.LastLoadId = utils.NOT_AVAILABLE
		cs.LastLoadTime = utils.NOT_AVAILABLE
	} else {
		cs.LastLoadId = loadHistInsts[0].LoadId
		cs.LastLoadTime = loadHistInsts[0].LoadTime.Format(time.RFC3339)
	}
	*reply = *cs
	return nil
}

func (self *ApierV1) GetCachedItemAge(itemId string, reply *utils.CachedItemAge) error {
	if len(itemId) == 0 {
		return fmt.Errorf("%s:ItemId", utils.ErrMandatoryIeMissing.Error())
	}
	cachedItemAge := new(utils.CachedItemAge)
	var found bool
	for idx, cacheKey := range []string{utils.DESTINATION_PREFIX + itemId, utils.RATING_PLAN_PREFIX + itemId, utils.RATING_PROFILE_PREFIX + itemId,
		utils.ACTION_PREFIX + itemId, utils.SHARED_GROUP_PREFIX + itemId, utils.ALIASES_PREFIX + itemId, utils.LCR_PREFIX + itemId} {

		if age, err := cache2go.GetKeyAge(cacheKey); err == nil {
			found = true
			switch idx {
			case 0:
				cachedItemAge.Destination = age
			case 1:
				cachedItemAge.RatingPlan = age
			case 2:
				cachedItemAge.RatingProfile = age
			case 3:
				cachedItemAge.Action = age
			case 4:
				cachedItemAge.SharedGroup = age
			case 5:
				cachedItemAge.Alias = age
			case 6:
				cachedItemAge.LcrProfiles = age
			}
		}
	}
	if !found {
		return utils.ErrNotFound
	}
	*reply = *cachedItemAge
	return nil
}

func (self *ApierV1) LoadTariffPlanFromFolder(attrs utils.AttrLoadTpFromFolder, reply *string) error {
	if len(attrs.FolderPath) == 0 {
		return fmt.Errorf("%s:%s", utils.ErrMandatoryIeMissing.Error(), "FolderPath")
	}
	if fi, err := os.Stat(attrs.FolderPath); err != nil {
		if strings.HasSuffix(err.Error(), "no such file or directory") {
			return utils.ErrInvalidPath
		}
		return utils.NewErrServerError(err)
	} else if !fi.IsDir() {
		return utils.ErrInvalidPath
	}
	loader := engine.NewTpReader(self.RatingDb, self.AccountDb, engine.NewFileCSVStorage(utils.CSV_SEP,
		path.Join(attrs.FolderPath, utils.DESTINATIONS_CSV),
		path.Join(attrs.FolderPath, utils.TIMINGS_CSV),
		path.Join(attrs.FolderPath, utils.RATES_CSV),
		path.Join(attrs.FolderPath, utils.DESTINATION_RATES_CSV),
		path.Join(attrs.FolderPath, utils.RATING_PLANS_CSV),
		path.Join(attrs.FolderPath, utils.RATING_PROFILES_CSV),
		path.Join(attrs.FolderPath, utils.SHARED_GROUPS_CSV),
		path.Join(attrs.FolderPath, utils.LCRS_CSV),
		path.Join(attrs.FolderPath, utils.ACTIONS_CSV),
		path.Join(attrs.FolderPath, utils.ACTION_PLANS_CSV),
		path.Join(attrs.FolderPath, utils.ACTION_TRIGGERS_CSV),
		path.Join(attrs.FolderPath, utils.ACCOUNT_ACTIONS_CSV),
		path.Join(attrs.FolderPath, utils.DERIVED_CHARGERS_CSV),
		path.Join(attrs.FolderPath, utils.CDR_STATS_CSV),
		path.Join(attrs.FolderPath, utils.USERS_CSV),
		path.Join(attrs.FolderPath, utils.ALIASES_CSV),
	), "", self.Config.DefaultTimezone, self.Config.LoadHistorySize)
	if err := loader.LoadAll(); err != nil {
		return utils.NewErrServerError(err)
	}
	if attrs.DryRun {
		*reply = OK
		return nil // Mission complete, no errors
	}

	if attrs.Validate {
		if !loader.IsValid() {
			*reply = OK
			return errors.New("invalid data")
		}
	}

	if err := loader.WriteToDatabase(attrs.FlushDb, false); err != nil {
		return utils.NewErrServerError(err)
	}
	// Make sure the items are in the cache
	dstIds, _ := loader.GetLoadedIds(utils.DESTINATION_PREFIX)
	dstKeys := make([]string, len(dstIds))
	for idx, dId := range dstIds {
		dstKeys[idx] = utils.DESTINATION_PREFIX + dId // Cache expects them as redis keys
	}
	rplIds, _ := loader.GetLoadedIds(utils.RATING_PLAN_PREFIX)
	rpKeys := make([]string, len(rplIds))
	for idx, rpId := range rplIds {
		rpKeys[idx] = utils.RATING_PLAN_PREFIX + rpId
	}
	rpfIds, _ := loader.GetLoadedIds(utils.RATING_PROFILE_PREFIX)
	rpfKeys := make([]string, len(rpfIds))
	for idx, rpfId := range rpfIds {
		rpfKeys[idx] = utils.RATING_PROFILE_PREFIX + rpfId
	}
	actIds, _ := loader.GetLoadedIds(utils.ACTION_PREFIX)
	actKeys := make([]string, len(actIds))
	for idx, actId := range actIds {
		actKeys[idx] = utils.ACTION_PREFIX + actId
	}
	shgIds, _ := loader.GetLoadedIds(utils.SHARED_GROUP_PREFIX)
	shgKeys := make([]string, len(shgIds))
	for idx, shgId := range shgIds {
		shgKeys[idx] = utils.SHARED_GROUP_PREFIX + shgId
	}
	aliases, _ := loader.GetLoadedIds(utils.ALIASES_PREFIX)
	alsKeys := make([]string, len(aliases))
	for idx, alias := range aliases {
		alsKeys[idx] = utils.ALIASES_PREFIX + alias
	}
	lcrIds, _ := loader.GetLoadedIds(utils.LCR_PREFIX)
	lcrKeys := make([]string, len(lcrIds))
	for idx, lcrId := range lcrIds {
		lcrKeys[idx] = utils.LCR_PREFIX + lcrId
	}
	dcs, _ := loader.GetLoadedIds(utils.DERIVEDCHARGERS_PREFIX)
	dcsKeys := make([]string, len(dcs))
	for idx, dc := range dcs {
		dcsKeys[idx] = utils.DERIVEDCHARGERS_PREFIX + dc
	}
	aps, _ := loader.GetLoadedIds(utils.ACTION_TIMING_PREFIX)
	utils.Logger.Info("ApierV1.LoadTariffPlanFromFolder, reloading cache.")

	if err := self.RatingDb.CacheRatingPrefixValues(map[string][]string{
		utils.DESTINATION_PREFIX:     dstKeys,
		utils.RATING_PLAN_PREFIX:     rpKeys,
		utils.RATING_PROFILE_PREFIX:  rpfKeys,
		utils.LCR_PREFIX:             lcrKeys,
		utils.DERIVEDCHARGERS_PREFIX: dcsKeys,
		utils.ACTION_PREFIX:          actKeys,
		utils.SHARED_GROUP_PREFIX:    shgKeys,
	}); err != nil {
		return err
	}
	if err := self.AccountDb.CacheAccountingPrefixValues(map[string][]string{
		utils.ALIASES_PREFIX: alsKeys,
	}); err != nil {
		return err
	}
	if len(aps) != 0 && self.Sched != nil {
		utils.Logger.Info("ApierV1.LoadTariffPlanFromFolder, reloading scheduler.")
		self.Sched.LoadActionPlans(self.RatingDb)
		self.Sched.Restart()
	}
	cstKeys, _ := loader.GetLoadedIds(utils.CDR_STATS_PREFIX)
	if len(cstKeys) != 0 && self.CdrStatsSrv != nil {
		if err := self.CdrStatsSrv.ReloadQueues(cstKeys, nil); err != nil {
			return err
		}
	}
	userKeys, _ := loader.GetLoadedIds(utils.USERS_PREFIX)
	if len(userKeys) != 0 && self.Users != nil {
		var r string
		if err := self.Users.ReloadUsers("", &r); err != nil {
			return err
		}
	}
	*reply = utils.OK
	return nil
}

type AttrRemoveRatingProfile struct {
	Direction string
	Tenant    string
	Category  string
	Subject   string
}

func (arrp *AttrRemoveRatingProfile) GetId() (result string) {
	if arrp.Direction != "" && arrp.Direction != utils.ANY {
		result += arrp.Direction
		result += utils.CONCATENATED_KEY_SEP
	} else {
		return
	}
	if arrp.Tenant != "" && arrp.Tenant != utils.ANY {
		result += arrp.Tenant
		result += utils.CONCATENATED_KEY_SEP
	} else {
		return
	}

	if arrp.Category != "" && arrp.Category != utils.ANY {
		result += arrp.Category
		result += utils.CONCATENATED_KEY_SEP
	} else {
		return
	}
	if arrp.Subject != "" && arrp.Subject != utils.ANY {
		result += arrp.Subject
	}
	return
}

func (self *ApierV1) RemoveRatingProfile(attr AttrRemoveRatingProfile, reply *string) error {
	log.Printf("ATTR: %+v", attr)
	if attr.Direction == "" {
		attr.Direction = utils.OUT
	}
	if (attr.Subject != "" && utils.IsSliceMember([]string{attr.Direction, attr.Tenant, attr.Category}, "")) ||
		(attr.Category != "" && utils.IsSliceMember([]string{attr.Direction, attr.Tenant}, "")) ||
		attr.Tenant != "" && attr.Direction == "" {
		return utils.ErrMandatoryIeMissing
	}
	_, err := engine.Guardian.Guard(func() (interface{}, error) {
		log.Print("RPID: ", attr.GetId())
		err := self.RatingDb.RemoveRatingProfile(attr.GetId())
		if err != nil {
			return 0, err
		}
		return 0, nil
	}, 0, "RemoveRatingProfile")
	if err != nil {
		*reply = err.Error()
		return utils.NewErrServerError(err)
	}
	*reply = utils.OK
	return nil
}

func (self *ApierV1) GetLoadHistory(attrs utils.Paginator, reply *[]*engine.LoadInstance) error {
	nrItems := -1
	offset := 0
	if attrs.Offset != nil { // For offset we need full data
		offset = *attrs.Offset
	} else if attrs.Limit != nil {
		nrItems = *attrs.Limit
	}
	loadHist, err := self.AccountDb.GetLoadHistory(nrItems, true)
	if err != nil {
		return utils.NewErrServerError(err)
	}
	if attrs.Offset != nil && attrs.Limit != nil { // Limit back to original
		nrItems = *attrs.Limit
	}
	if len(loadHist) == 0 || len(loadHist) <= offset || nrItems == 0 {
		return utils.ErrNotFound
	}
	if offset != 0 {
		nrItems = offset + nrItems
	}
	if nrItems == -1 || nrItems > len(loadHist) { // So we can use it in indexing bellow
		nrItems = len(loadHist)
	}
	*reply = loadHist[offset:nrItems]
	return nil
}
