import SwiftUI

/// „Meine Vermietung" — who is asking for my devices, which devices are mine,
/// and which stretches I keep for myself.
///
/// Reachable only from the profile, and only when the platform has approved
/// somebody as a lender. Adding or changing a device is **not** here: that
/// happens in the chat and the web version of the Maschinchenring, and it
/// stays there.
struct RentalOwnerView: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    @State private var model: RentalOwnerModel?

    var body: some View {
        Group {
            if let model {
                RentalOwnerList(model: model)
            } else {
                ProgressView().controlSize(.large)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .navigationTitle("Meine Vermietung")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            let existing = model ?? RentalOwnerModel(source: .from(umgebung.rental))
            model = existing
            await existing.load()
        }
    }
}

struct RentalOwnerList: View {
    @ObservedObject var model: RentalOwnerModel
    /// The device a new block is being drawn for.
    @State private var blockingDevice: RentalDevice?

    var body: some View {
        List {
            if let trouble = model.trouble, trouble.wantsSignIn {
                Section {
                    RentalSignInNotice(trouble: trouble) {
                        model.clearTrouble()
                        await model.refresh()
                    }
                }
            } else if let hint = model.hint {
                Section {
                    RentalNotice(text: hint) { Task { await model.refresh() } }
                }
            }

            bookingsSection
            devicesSection
            blocksSection
        }
        .accessibilityIdentifier("rental-owner")
        .refreshable { await model.refresh() }
        .sheet(item: $blockingDevice) { device in
            RentalBlockSheet(model: model, device: device)
        }
    }

    private var bookingsSection: some View {
        Section {
            if model.loading && model.bookings.isEmpty {
                HStack(spacing: 10) {
                    ProgressView()
                    Text("Anfragen werden geholt …").foregroundStyle(.secondary)
                }
            }

            ForEach(model.bookings) { booking in
                RentalOwnerBookingRow(model: model, booking: booking)
            }

            if model.bookings.isEmpty && !model.loading {
                Text("Für deine Geräte liegt gerade nichts vor.")
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("rental-owner-no-bookings")
            }
        } header: {
            Text(model.waiting > 0 ? "Anfragen (\(model.waiting) offen)" : "Anfragen")
        } footer: {
            Text("Sobald du zusagst, bekommt der Mieter deine Abholadresse per "
                + "E-Mail — vorher nicht.")
        }
    }

    private var devicesSection: some View {
        Section {
            ForEach(model.devices) { device in
                HStack {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(device.name).font(.subheadline.weight(.medium))
                        if device.active == false {
                            Text("Abgeschaltet — für andere unsichtbar.")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                    Spacer()
                    Button("Sperren") { blockingDevice = device }
                        .buttonStyle(.borderless)
                        .accessibilityIdentifier("rental-block-\(device.id)")
                }
                .accessibilityIdentifier("rental-owner-device-\(device.id)")
            }

            if model.devices.isEmpty && !model.loading {
                Text("Du hast noch kein Gerät eingestellt.")
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("rental-owner-no-devices")
            }
        } header: {
            Text("Meine Geräte")
        } footer: {
            Text("Geräte anlegen und ändern geht im Maschinchenring selbst — "
                + "in der App bewusst nicht.")
        }
    }

    @ViewBuilder private var blocksSection: some View {
        Section("Meine Sperren") {
            ForEach(model.blocks) { block in
                VStack(alignment: .leading, spacing: 3) {
                    Text(block.deviceName).font(.subheadline.weight(.medium))
                    Label(block.periodText, systemImage: "calendar")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                    if let reason = block.reason {
                        Text(reason).font(.footnote).foregroundStyle(.secondary)
                    }
                    Button(role: .destructive) {
                        Task { await model.removeBlock(block) }
                    } label: {
                        Text("Sperre aufheben")
                    }
                    .buttonStyle(.borderless)
                    .accessibilityIdentifier("rental-unblock-\(block.id)")
                }
                .padding(.vertical, 2)
            }

            if model.blocks.isEmpty && !model.loading {
                Text("Keine Sperre eingetragen.")
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("rental-owner-no-blocks")
            }
        }
    }
}

/// One request on one of my devices — with the two decisions the platform
/// says are possible right now.
struct RentalOwnerBookingRow: View {
    @ObservedObject var model: RentalOwnerModel
    let booking: RentalOwnerBooking

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(booking.deviceName).font(.headline)
            Label(booking.periodText, systemImage: "calendar")
                .font(.subheadline)
                .foregroundStyle(.secondary)
            if let back = booking.returnText {
                Text(back).font(.footnote).foregroundStyle(.secondary)
            }
            Label(booking.statusLabel, systemImage: booking.state.symbol)
                .font(.subheadline.weight(.medium))

            // Name and telephone are here so the handover can be arranged,
            // and they appear nowhere else in the app.
            if let name = booking.renterName {
                Label(name, systemImage: "person").font(.footnote)
            }
            if let phone = booking.renterPhone {
                Label(phone, systemImage: "phone").font(.footnote)
            }
            if let note = booking.notes {
                Text(note).font(.footnote).foregroundStyle(.secondary)
            }

            // The buttons follow `canDecide` and `canCancel`. Whether a
            // decision is still open is the platform's answer.
            HStack(spacing: 16) {
                if model.deciding.contains(booking.id) {
                    ProgressView().controlSize(.small)
                }
                if booking.canDecide {
                    Button("Zusagen") { Task { await model.approve(booking) } }
                        .buttonStyle(.borderless)
                        .accessibilityIdentifier("rental-approve-\(booking.id)")
                    Button("Absagen", role: .destructive) {
                        Task { await model.reject(booking) }
                    }
                    .buttonStyle(.borderless)
                    .accessibilityIdentifier("rental-reject-\(booking.id)")
                } else if booking.canCancel {
                    Button("Stornieren", role: .destructive) {
                        Task { await model.cancel(booking) }
                    }
                    .buttonStyle(.borderless)
                    .accessibilityIdentifier("rental-owner-cancel-\(booking.id)")
                }
            }
            .disabled(model.deciding.contains(booking.id))
        }
        .padding(.vertical, 2)
        .accessibilityIdentifier("rental-owner-booking-\(booking.id)")
    }
}

/// „Zeitraum sperren" — the lender keeps a stretch for themselves.
struct RentalBlockSheet: View {
    @ObservedObject var model: RentalOwnerModel
    let device: RentalDevice
    @Environment(\.dismiss) private var dismiss

    @State private var firstDay = Date()
    @State private var lastDay = Date()
    @State private var reason = ""

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    DatePicker("Von", selection: $firstDay, displayedComponents: .date)
                        .accessibilityIdentifier("rental-block-first-day")
                    DatePicker("Bis einschließlich", selection: $lastDay,
                               in: firstDay..., displayedComponents: .date)
                        .accessibilityIdentifier("rental-block-last-day")
                    TextField("Grund (freiwillig)", text: $reason)
                        .accessibilityIdentifier("rental-block-reason")
                } header: {
                    Text(device.name)
                } footer: {
                    Text("Für andere sieht die Sperre aus wie jeder belegte "
                        + "Zeitraum — ohne Grund und ohne Namen. Eine bestehende "
                        + "Buchung verdrängt sie nicht.")
                }

                if let trouble = model.trouble {
                    Section {
                        Text(trouble.message)
                            .font(.footnote)
                            .accessibilityIdentifier("rental-block-trouble")
                    }
                }
            }
            .navigationTitle("Zeitraum sperren")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Abbrechen") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Sperren") {
                        Task {
                            let done = await model.block(
                                deviceId: device.id, firstDay: firstDay,
                                lastDay: lastDay, reason: reason
                            )
                            if done { dismiss() }
                        }
                    }
                    .disabled(model.blocking)
                    .accessibilityIdentifier("rental-block-save")
                }
            }
        }
    }
}
