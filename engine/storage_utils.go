/*
Rating system designed to be used in VoIP Carriers World
Copyright (C) 2012-2015 ITsysCOM

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

package engine

import (
	"errors"
	"strconv"

	"github.com/cgrates/cgrates/utils"
)

// Various helpers to deal with database

func ConfigureRatingStorage(db_type, host, port, name, user, pass, marshaler string) (db RatingStorage, err error) {
	var d Storage
	switch db_type {
	case utils.REDIS:
		var db_nb int
		db_nb, err = strconv.Atoi(name)
		if err != nil {
			utils.Logger.Crit("Redis db name must be an integer!")
			return nil, err
		}
		if port != "" {
			host += ":" + port
		}
		d, err = NewRedisStorage(host, db_nb, pass, marshaler)
	default:
		err = errors.New("unknown db")
	}
	if err != nil {
		return nil, err
	}
	return d.(RatingStorage), nil
}

func ConfigureAccountingStorage(db_type, host, port, name, user, pass, marshaler string) (db AccountingStorage, err error) {
	var d Storage
	switch db_type {
	case utils.REDIS:
		var db_nb int
		db_nb, err = strconv.Atoi(name)
		if err != nil {
			utils.Logger.Crit("Redis db name must be an integer!")
			return nil, err
		}
		if port != "" {
			host += ":" + port
		}
		d, err = NewRedisStorage(host, db_nb, pass, marshaler)
	/*
		case utils.MONGO:
			d, err = NewMongoStorage(host, port, name, user, pass)
			db = d.(AccountingStorage)
	*/
	default:
		err = errors.New("unknown db")
	}
	if err != nil {
		return nil, err
	}
	return d.(AccountingStorage), nil
}

func ConfigureLogStorage(db_type, host, port, name, user, pass, marshaler string, maxConn, maxIdleConn int) (db LogStorage, err error) {
	var d Storage
	switch db_type {
	/*
		case utils.REDIS:
			var db_nb int
			db_nb, err = strconv.Atoi(name)
			if err != nil {
				utils.Logger.Crit("Redis db name must be an integer!")
				return nil, err
			}
			if port != "" {
				host += ":" + port
			}
			d, err = NewRedisStorage(host, db_nb, pass, marshaler)
		case utils.MONGO:
			d, err = NewMongoStorage(host, port, name, user, pass)
	*/
	case utils.POSTGRES:
		d, err = NewPostgresStorage(host, port, name, user, pass, maxConn, maxIdleConn)
	case utils.MYSQL:
		d, err = NewMySQLStorage(host, port, name, user, pass, maxConn, maxIdleConn)
	default:
		err = errors.New("unknown db")
	}
	if err != nil {
		return nil, err
	}
	return d.(LogStorage), nil
}

func ConfigureLoadStorage(db_type, host, port, name, user, pass, marshaler string, maxConn, maxIdleConn int) (db LoadStorage, err error) {
	var d Storage
	switch db_type {
	case utils.POSTGRES:
		d, err = NewPostgresStorage(host, port, name, user, pass, maxConn, maxIdleConn)
	case utils.MYSQL:
		d, err = NewMySQLStorage(host, port, name, user, pass, maxConn, maxIdleConn)
	default:
		err = errors.New("unknown db")
	}
	if err != nil {
		return nil, err
	}
	return d.(LoadStorage), nil
}

func ConfigureCdrStorage(db_type, host, port, name, user, pass string, maxConn, maxIdleConn int) (db CdrStorage, err error) {
	var d Storage
	switch db_type {
	case utils.POSTGRES:
		d, err = NewPostgresStorage(host, port, name, user, pass, maxConn, maxIdleConn)
	case utils.MYSQL:
		d, err = NewMySQLStorage(host, port, name, user, pass, maxConn, maxIdleConn)
	default:
		err = errors.New("unknown db")
	}
	if err != nil {
		return nil, err
	}
	return d.(CdrStorage), nil
}
