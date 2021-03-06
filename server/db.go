package server

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"time"

	"go.uber.org/zap"

	"cloud.google.com/go/firestore"
	"github.com/lietu/godometer"
)

const debugDb = false

var utc, _ = time.LoadLocation("UTC")

type LastEventContainer struct {
	Events []ResponseDataPoint `firestore:"events"`
}

func collectionName(period string) string {
	return fmt.Sprintf("godometer-%s-records", period)
}

func recordStr(record DBDataPoint) string {
	return fmt.Sprintf("%.2fm @ %.1fm/s or %.1fkm/h (%d records)", record.Meters, record.MetersPerSecond, record.KilometersPerHour, record.Counter)
}

func printRecords(records map[string]DBDataPoint) {
	var keys []string
	for key := range records {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		row := records[key]
		log.Printf("%s: %s", key, recordStr(row))
	}
}

func latestKey(records map[string]DBDataPoint) string {
	var keys []string
	for key := range records {
		keys = append(keys, key)
	}

	if len(keys) == 0 {
		return ""
	}

	sort.Strings(keys)

	return keys[len(keys)-1]
}

func (s *Server) printAllRecords() {
	log.Print(" ----- RECORDS IN MEMORY -----")
	log.Print(" ----- MINUTE RECORDS -----")
	printRecords(s.minutes)
	log.Print(" ----- HOUR RECORDS -----")
	printRecords(s.hours)
	log.Print(" ----- DAY RECORDS -----")
	printRecords(s.days)
	log.Print(" ----- WEEK RECORDS -----")
	printRecords(s.weeks)
	log.Print(" ----- MONTH RECORDS -----")
	printRecords(s.months)
	log.Print(" ----- YEAR RECORDS -----")
	printRecords(s.years)
}

func (s *Server) printLatestRecords() {
	log.Printf("----- LATEST RECORDS -----")
	log.Printf("Latest minute: %s", recordStr(s.minutes[latestKey(s.minutes)]))
	log.Printf("Latest hour:   %s", recordStr(s.hours[latestKey(s.hours)]))
	log.Printf("Latest day:    %s", recordStr(s.days[latestKey(s.days)]))
	log.Printf("Latest week:   %s", recordStr(s.weeks[latestKey(s.weeks)]))
	log.Printf("Latest month:  %s", recordStr(s.months[latestKey(s.months)]))
	log.Printf("Latest year:   %s", recordStr(s.years[latestKey(s.years)]))
}

func (s *Server) loadData() {
	// Initialize all data structures
	minutes := Last60Minutes()
	hours := Last24Hours()
	days := Last7Days()
	weeks := Last5Weeks()
	months := Last12Months()
	years := Last4Years()

	s.minutes = map[string]DBDataPoint{}
	for _, key := range minutes {
		s.minutes[key] = DBDataPoint{
			Meters:            0.0,
			MetersPerSecond:   0.0,
			KilometersPerHour: 0.0,
		}
	}

	s.hours = map[string]DBDataPoint{}
	for _, key := range hours {
		s.hours[key] = DBDataPoint{
			Meters:            0.0,
			MetersPerSecond:   0.0,
			KilometersPerHour: 0.0,
		}
	}

	s.days = map[string]DBDataPoint{}
	for _, key := range days {
		s.days[key] = DBDataPoint{
			Meters:            0.0,
			MetersPerSecond:   0.0,
			KilometersPerHour: 0.0,
		}
	}

	s.weeks = map[string]DBDataPoint{}
	for _, key := range weeks {
		s.weeks[key] = DBDataPoint{
			Meters:            0.0,
			MetersPerSecond:   0.0,
			KilometersPerHour: 0.0,
		}
	}

	s.months = map[string]DBDataPoint{}
	for _, key := range months {
		s.months[key] = DBDataPoint{
			Meters:            0.0,
			MetersPerSecond:   0.0,
			KilometersPerHour: 0.0,
		}
	}

	s.years = map[string]DBDataPoint{}
	for _, key := range years {
		s.years[key] = DBDataPoint{
			Meters:            0.0,
			MetersPerSecond:   0.0,
			KilometersPerHour: 0.0,
		}
	}

	ctx := context.Background()
	s.readEvents(ctx)
	s.readYears(ctx, years[:])
	s.readMonths(ctx, months[:])
	s.readWeeks(ctx, weeks[:])
	s.readDays(ctx, days[:])
	s.readHours(ctx, hours[:])
	s.readMinutes(ctx, minutes[:])
}

func (s *Server) readEvents(ctx context.Context) {
	s.lastEvents = []ResponseDataPoint{}

	db := GetClient(ctx, s.projectId)
	eventsColl := db.Collection(collectionName("events"))
	ref := eventsColl.Doc("lastEvents")
	doc, err := ref.Get(ctx)
	if err != nil {
		logger.Warn("Got error trying to load past events", zap.Error(err))
		return
	}

	eventContainer := LastEventContainer{}
	err = doc.DataTo(&eventContainer)
	if err != nil {
		logger.Warn("Got error trying to parse past events", zap.Error(err))
		return
	}

	s.lastEvents = eventContainer.Events

	if debugDb {
		log.Printf("Recent events")
		for _, e := range s.lastEvents {
			log.Printf("%s: %.1fm @ %.1fm/s or %.1fkm/h", e.Timestamp, e.Meters, e.MetersPerSecond, e.KilometersPerHour)
		}
	}
}

func (s *Server) readRecords(ctx context.Context, collection string, ids []string) map[string]DBDataPoint {
	db := GetClient(ctx, s.projectId)
	collRef := db.Collection(collection)
	var refs []*firestore.DocumentRef
	for _, id := range ids {
		refs = append(refs, collRef.Doc(id))
	}

	results, err := db.GetAll(ctx, refs)
	if err != nil {
		logger.Warn("Error fetching records from DB", zap.Error(err))
	}

	records := map[string]DBDataPoint{}
	for _, r := range results {
		row := DBDataPoint{
			Meters:            0.0,
			MetersPerSecond:   0.0,
			KilometersPerHour: 0.0,
		}

		// Non-existing rows will be zeroed out, this is ok
		if r.Exists() {
			err := r.DataTo(&row)
			if err != nil {
				logger.Warn("Failed to read data from DB to record. This is probably not great.", zap.Error(err))
			}
		}
		records[r.Ref.ID] = row
	}

	return records
}

func (s *Server) readYears(ctx context.Context, years []string) {
	s.years = s.readRecords(ctx, collectionName("years"), years)
}

func (s *Server) readMonths(ctx context.Context, months []string) {
	s.months = s.readRecords(ctx, collectionName("months"), months)
}

func (s *Server) readWeeks(ctx context.Context, weeks []string) {
	s.weeks = s.readRecords(ctx, collectionName("weeks"), weeks)
}

func (s *Server) readDays(ctx context.Context, days []string) {
	s.days = s.readRecords(ctx, collectionName("days"), days)
}

func (s *Server) readHours(ctx context.Context, hours []string) {
	s.hours = s.readRecords(ctx, collectionName("hours"), hours)
}

func (s *Server) readMinutes(ctx context.Context, minutes []string) {
	s.minutes = s.readRecords(ctx, collectionName("minutes"), minutes)
}

func stringInList(items []string, item string) bool {
	for _, i := range items {
		if i == item {
			return true
		}
	}

	return false
}

func (s *Server) clearOldStats() {
	// List of data we want to store
	minutes := Last60Minutes()
	hours := Last24Hours()
	days := Last7Days()
	weeks := Last5Weeks()
	months := Last12Months()
	years := Last4Years()

	// Create any missing keys
	for _, key := range minutes {
		if _, ok := s.minutes[key]; !ok {
			s.minutes[key] = DBDataPoint{
				Counter:           0,
				Meters:            0.0,
				MetersPerSecond:   0.0,
				KilometersPerHour: 0.0,
			}
		}
	}

	for _, key := range hours {
		if _, ok := s.hours[key]; !ok {
			s.hours[key] = DBDataPoint{
				Counter:           0,
				Meters:            0.0,
				MetersPerSecond:   0.0,
				KilometersPerHour: 0.0,
			}
		}
	}

	for _, key := range days {
		if _, ok := s.days[key]; !ok {
			s.days[key] = DBDataPoint{
				Counter:           0,
				Meters:            0.0,
				MetersPerSecond:   0.0,
				KilometersPerHour: 0.0,
			}
		}
	}

	for _, key := range weeks {
		if _, ok := s.weeks[key]; !ok {
			s.weeks[key] = DBDataPoint{
				Counter:           0,
				Meters:            0.0,
				MetersPerSecond:   0.0,
				KilometersPerHour: 0.0,
			}
		}
	}

	for _, key := range months {
		if _, ok := s.months[key]; !ok {
			s.months[key] = DBDataPoint{
				Counter:           0,
				Meters:            0.0,
				MetersPerSecond:   0.0,
				KilometersPerHour: 0.0,
			}
		}
	}

	for _, key := range years {
		if _, ok := s.years[key]; !ok {
			s.years[key] = DBDataPoint{
				Counter:           0,
				Meters:            0.0,
				MetersPerSecond:   0.0,
				KilometersPerHour: 0.0,
			}
		}
	}

	// Strip out any extra ones
	for key := range s.minutes {
		if !stringInList(minutes[:], key) {
			delete(s.minutes, key)
		}
	}

	for key := range s.hours {
		if !stringInList(hours[:], key) {
			delete(s.hours, key)
		}
	}

	for key := range s.days {
		if !stringInList(days[:], key) {
			delete(s.days, key)
		}
	}

	for key := range s.weeks {
		if !stringInList(weeks[:], key) {
			delete(s.weeks, key)
		}
	}

	for key := range s.months {
		if !stringInList(months[:], key) {
			delete(s.months, key)
		}
	}

	for key := range s.years {
		if !stringInList(years[:], key) {
			delete(s.years, key)
		}
	}
}

func calculateUpdate(old DBDataPoint, ok bool, newRow DBDataPoint) (DBDataPoint, bool) {
	result := newRow
	save := false

	if ok {
		totalMPS := (old.MetersPerSecond * float32(old.Counter)) + newRow.MetersPerSecond
		totalKPH := (old.KilometersPerHour * float32(old.Counter)) + newRow.KilometersPerHour

		result = DBDataPoint{}
		// Only count updates with actual data in them
		if newRow.Meters > 0 && newRow.MetersPerSecond > 0 && newRow.KilometersPerHour > 0 {
			result.Counter = old.Counter + 1
			save = true
		}

		result.Meters = old.Meters + newRow.Meters

		if result.Counter > 0 {
			result.MetersPerSecond = totalMPS / float32(result.Counter)
			result.KilometersPerHour = totalKPH / float32(result.Counter)
		} else {
			result.MetersPerSecond = 0
			result.KilometersPerHour = 0
		}
	} else {
		save = true
	}

	return result, save
}

func (s *Server) isKnownEvent(dataPoint godometer.UpdateDataPoint) bool {
	for _, dp := range s.lastEvents {
		if dp.Timestamp == dataPoint.Timestamp {
			return true
		}
	}

	return false
}

func (s *Server) cleanLastEvents() {
	max := 5
	current := len(s.lastEvents)
	keep := 0

	if current > max {
		keep = current - max
	}

	s.lastEvents = s.lastEvents[keep:]
}

func (s *Server) writeStats(ctx context.Context, updateDataPoints []godometer.UpdateDataPoint) {
	var years []string
	var months []string
	var weeks []string
	var days []string
	var hours []string
	var minutes []string
	var newEvents []string

	newDataPoints := 0
	for _, udp := range updateDataPoints {
		// Ignore already processed events
		if s.isKnownEvent(udp) {
			continue
		}

		currentDataPoint := DBDataPoint{
			Counter:           1,
			Meters:            udp.Meters,
			MetersPerSecond:   udp.MetersPerSecond,
			KilometersPerHour: udp.KilometersPerHour,
		}

		ts, err := time.Parse(minuteLayout, udp.Timestamp)
		if err != nil {
			logger.Warn("Failed to parse time", zap.String("timestamp", udp.Timestamp), zap.Error(err))
			continue
		}

		year := ts.Format(yearLayout)
		month := ts.Format(monthLayout)
		week := weekFormat(ts)
		day := ts.Format(dayLayout)
		hour := ts.Format(hourLayout)
		minute := ts.Format(minuteLayout)

		yearRow, yearsOk := s.years[year]
		monthRow, monthsOk := s.months[month]
		weekRow, weeksOk := s.weeks[week]
		dayRow, daysOk := s.days[day]
		hourRow, hoursOk := s.hours[hour]
		_, minutesOk := s.minutes[minute]

		yearRow, saveYear := calculateUpdate(yearRow, yearsOk, currentDataPoint)
		monthRow, saveMonth := calculateUpdate(monthRow, monthsOk, currentDataPoint)
		weekRow, saveWeek := calculateUpdate(weekRow, weeksOk, currentDataPoint)
		dayRow, saveDay := calculateUpdate(dayRow, daysOk, currentDataPoint)
		hourRow, saveHour := calculateUpdate(hourRow, hoursOk, currentDataPoint)
		saveMinute := false
		if currentDataPoint.Meters > 0 || currentDataPoint.MetersPerSecond > 0 || currentDataPoint.KilometersPerHour > 0 || minutesOk {
			saveMinute = true
		}

		if saveYear && !stringInList(years, year) {
			years = append(years, year)
		}

		if saveMonth && !stringInList(months, month) {
			months = append(months, month)
		}

		if saveWeek && !stringInList(weeks, week) {
			weeks = append(weeks, week)
		}

		if saveDay && !stringInList(days, day) {
			days = append(days, day)
		}

		if saveHour && !stringInList(hours, hour) {
			hours = append(hours, hour)
		}

		if saveMinute && !stringInList(minutes, minute) {
			minutes = append(minutes, minute)
		}

		s.years[year] = yearRow
		s.months[month] = monthRow
		s.weeks[week] = weekRow
		s.days[day] = dayRow
		s.hours[hour] = hourRow
		s.minutes[minute] = currentDataPoint

		s.lastEvents = append(s.lastEvents, currentDataPoint.toResponseDataPoint(udp.Timestamp))
		newDataPoints += 1
		newEvents = append(newEvents, udp.Timestamp)
	}

	s.cleanLastEvents()

	db := GetClient(ctx, s.projectId)
	batch := db.Batch()

	eventsColl := db.Collection(collectionName("events"))
	yearsColl := db.Collection(collectionName("years"))
	monthsColl := db.Collection(collectionName("months"))
	weeksColl := db.Collection(collectionName("weeks"))
	daysColl := db.Collection(collectionName("days"))
	hoursColl := db.Collection(collectionName("hours"))
	minutesColl := db.Collection(collectionName("minutes"))

	batchRecords := 0

	if newDataPoints > 0 {
		batchRecords += 1
		eventContainer := LastEventContainer{
			Events: s.lastEvents,
		}
		batch.Set(eventsColl.Doc("lastEvents"), eventContainer)
	}

	for _, id := range years {
		batchRecords += 1
		ref := yearsColl.Doc(id)
		batch.Set(ref, s.years[id])
	}

	for _, id := range months {
		batchRecords += 1
		ref := monthsColl.Doc(id)
		batch.Set(ref, s.months[id])
	}

	for _, id := range weeks {
		batchRecords += 1
		ref := weeksColl.Doc(id)
		batch.Set(ref, s.weeks[id])
	}

	for _, id := range days {
		batchRecords += 1
		ref := daysColl.Doc(id)
		batch.Set(ref, s.days[id])
	}

	for _, id := range hours {
		batchRecords += 1
		ref := hoursColl.Doc(id)
		batch.Set(ref, s.hours[id])
	}

	for _, id := range minutes {
		batchRecords += 1
		ref := minutesColl.Doc(id)
		batch.Set(ref, s.minutes[id])
	}

	if batchRecords > 0 {
		var keys []string
		keys = append(keys, years...)
		keys = append(keys, months...)
		keys = append(keys, weeks...)
		keys = append(keys, days...)
		keys = append(keys, hours...)
		keys = append(keys, minutes...)
		logger.Info("Processed events", zap.Strings("events", newEvents))
		logger.Info("Saving records to DB", zap.Int("count", batchRecords), zap.Strings("keys", keys))
		_, err := batch.Commit(ctx)
		if err != nil {
			logger.Warn("Error trying to save records to DB", zap.Error(err))
		}
	} else {
		logger.Info("How strange, no records updated")
	}

	s.clearOldStats()

	if debugDb {
		s.printLatestRecords()
	}
}

var firestoreClient *firestore.Client

func GetClient(ctx context.Context, projectId string) *firestore.Client {
	if firestoreClient == nil {
		c, err := firestore.NewClient(ctx, projectId)
		if err != nil {
			logger.Panic("Failed to connect to DB", zap.Error(err))
		}

		firestoreClient = c
	}

	return firestoreClient
}

func Last60Minutes() [60]string {
	var minutes [60]string
	step := time.Minute
	now := time.Now().In(utc)
	nextStr := now.Add(step).Format(minuteLayout)
	start := now.Add(-59 * step)

	current := start
	currentStr := current.Format(minuteLayout)

	index := 0
	for currentStr != nextStr {
		minutes[index] = currentStr
		current = current.Add(step)
		currentStr = current.Format(minuteLayout)
		index += 1
	}

	return minutes
}

func Last24Hours() [24]string {
	var hours [24]string
	step := time.Hour
	now := time.Now().In(utc)
	nextStr := now.Add(step).Format(hourLayout)
	start := now.Add(-23 * step)

	current := start
	currentStr := current.Format(hourLayout)

	index := 0
	for currentStr != nextStr {
		hours[index] = currentStr
		current = current.Add(step)
		currentStr = current.Format(hourLayout)
		index += 1
	}

	return hours
}

func Last7Days() [7]string {
	var days [7]string
	step := time.Hour * 24
	now := time.Now().In(utc)
	nextStr := now.Add(step).Format(dayLayout)
	start := now.Add(-6 * step)

	current := start
	currentStr := current.Format(dayLayout)

	index := 0
	for currentStr != nextStr {
		days[index] = currentStr
		current = current.Add(step)
		currentStr = current.Format(dayLayout)
		index += 1
	}

	return days
}

func Last5Weeks() [5]string {
	var weeks [5]string
	step := time.Hour * 24 * 7
	now := time.Now().In(utc)
	nextStr := weekFormat(now.Add(step))
	start := now.Add(-4 * step)

	current := start
	currentStr := weekFormat(current)

	index := 0
	for currentStr != nextStr {
		weeks[index] = currentStr
		current = current.Add(step)
		currentStr = weekFormat(current)
		index += 1
	}

	return weeks
}

func Last12Months() [12]string {
	var months [12]string
	now := time.Now().In(utc)
	nextStr := now.AddDate(0, 1, 0).Format(monthLayout)
	start := now.AddDate(0, -11, 0)

	current := start
	currentStr := current.Format(monthLayout)

	index := 0
	for currentStr != nextStr {
		months[index] = currentStr
		current = current.AddDate(0, 1, 0)
		currentStr = current.Format(monthLayout)
		index += 1
	}

	return months
}

func Last4Years() [4]string {
	var years [4]string
	now := time.Now().In(utc)
	nextStr := now.AddDate(1, 0, 0).Format(yearLayout)
	start := now.AddDate(-3, 0, 0)

	current := start
	currentStr := current.Format(yearLayout)

	index := 0
	for currentStr != nextStr {
		years[index] = currentStr
		current = current.AddDate(1, 0, 0)
		currentStr = current.Format(yearLayout)
		index += 1
	}

	return years
}

func fakeDataPoint() DBDataPoint {
	metersChange := rand.Float64() * 50.0
	if prevFakeMeters-metersChange > 0 && prevFakeMeters+metersChange < maxFakeMeters {
		dir := rand.Int31n(1) == 1
		if !dir {
			metersChange = -metersChange
		}
	} else if prevFakeMeters+metersChange > maxFakeMeters {
		metersChange = -metersChange
	}

	meters := prevFakeMeters + metersChange

	mps := float32(meters / 60.0)
	kph := mps * 3600.0 / 1000.0

	prevFakeMeters = meters

	return DBDataPoint{
		Counter:           1,
		Meters:            float32(meters),
		MetersPerSecond:   mps,
		KilometersPerHour: kph,
	}
}

func (s *Server) fillFakeDataRecords(records map[string]DBDataPoint) {
	for key := range records {
		records[key] = fakeDataPoint()
	}
}

func (s *Server) generateFakeData() {
	// Initialize all data structures
	s.fillFakeDataRecords(s.years)
	s.fillFakeDataRecords(s.months)
	s.fillFakeDataRecords(s.weeks)
	s.fillFakeDataRecords(s.days)
	s.fillFakeDataRecords(s.hours)
	s.fillFakeDataRecords(s.minutes)

	logger.Info("Filled records with fake data")

	tick := time.Tick(time.Minute)
	ctx := context.Background()
	for {
		select {
		case <-tick:
			dp := fakeDataPoint()
			udp := []godometer.UpdateDataPoint{
				{
					Timestamp:         time.Now().In(utc).Format(minuteLayout),
					Meters:            dp.Meters,
					MetersPerSecond:   dp.MetersPerSecond,
					KilometersPerHour: dp.KilometersPerHour,
				},
			}

			logger.Info("FAKED EVENT", zap.Float32("meters", udp[0].Meters), zap.Float32("MPS", udp[0].MetersPerSecond), zap.Float32("KPH", udp[0].KilometersPerHour))
			s.writeStats(ctx, udp)
		}
	}
}
